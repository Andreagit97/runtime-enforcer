package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/rancher-sandbox/runtime-enforcer/api/v1alpha1"
	"github.com/rancher-sandbox/runtime-enforcer/internal/grpcexporter"
	"github.com/rancher-sandbox/runtime-enforcer/internal/types/loglevel"
	"github.com/rancher-sandbox/runtime-enforcer/internal/types/policymode"
	pb "github.com/rancher-sandbox/runtime-enforcer/proto/agent/v1"

	otellog "go.opentelemetry.io/otel/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=security.rancher.io,resources=workloadpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.rancher.io,resources=workloadpolicies/status,verbs=get;update;patch

// WorkloadPolicyStatusSync reconciles a WorkloadPolicy status.
type WorkloadPolicyStatusSync struct {
	client.Client

	agentClientPool *grpcexporter.AgentClientPool
	updateInterval  time.Duration
	logger          logr.Logger
	eventLogger     otellog.Logger
}

// WorkloadPolicyStatusSyncConfig holds the configuration for the WorkloadPolicyStatusSync.
type WorkloadPolicyStatusSyncConfig struct {
	AgentPoolConf  grpcexporter.AgentClientPoolConfig
	UpdateInterval time.Duration
	EventLogger    otellog.Logger
}

func NewWorkloadPolicyStatusSync(
	c client.Client,
	config *WorkloadPolicyStatusSyncConfig,
) (*WorkloadPolicyStatusSync, error) {
	if config.UpdateInterval <= 0 {
		return nil, fmt.Errorf("invalid update interval: %v", config.UpdateInterval)
	}

	agentClientPool, err := grpcexporter.NewAgentClientPool(config.AgentPoolConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent client pool: %w", err)
	}

	return &WorkloadPolicyStatusSync{
		Client:          c,
		agentClientPool: agentClientPool,
		updateInterval:  config.UpdateInterval,
		eventLogger:     config.EventLogger,
	}, nil
}

func (r *WorkloadPolicyStatusSync) Start(ctx context.Context) error {
	r.logger = log.FromContext(ctx).WithName("WorkloadPolicyStatusSync")
	r.logger.Info("Starting with", "interval", r.updateInterval)
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Closing")
			return nil
		// today we keep this runnable single-threaded so after each sync we wait again `updateInterval`.
		case <-time.After(r.updateInterval):
			if err := r.sync(ctx); err != nil {
				r.logger.Error(err, "Failed to sync")
			}
		}
	}
}

func (r *WorkloadPolicyStatusSync) sync(
	ctx context.Context,
) error {
	// As first step, we list all WorkloadPolicies, if there are none, we can reschedule and exit early
	var wpList v1alpha1.WorkloadPolicyList
	if err := r.List(ctx, &wpList); err != nil {
		return err
	}

	if len(wpList.Items) == 0 {
		r.logger.V(loglevel.VerbosityDebug).Info("No WorkloadPolicies found, retrying later")
		return nil
	}

	clients, err := r.agentClientPool.UpdatePool(ctx, r.Client)
	if err != nil {
		return err
	}

	violationsByPolicy := r.getViolationsByPolicy(ctx, clients)
	nodeStatusByPolicy := r.getNodeStatusByPolicy(ctx, clients, wpList.Items)

	// Now we iterate over all WSPs and update their status based on the collected policies status from the agents
	for _, wp := range wpList.Items {
		wpNamespacedName := wp.NamespacedName()
		if err = r.processWorkloadPolicy(
			ctx,
			&wp,
			nodeStatusByPolicy[wpNamespacedName],
			violationsByPolicy[wpNamespacedName],
		); err != nil {
			r.logger.Error(
				err,
				"failed to process workload policy",
				"policy", wpNamespacedName,
			)
		}
	}

	return nil
}

// getViolationsByPolicy gets all the violations for a single policy.
func (r *WorkloadPolicyStatusSync) getViolationsByPolicy(
	ctx context.Context,
	clients map[string]grpcexporter.AgentClientAPI,
) map[string][]v1alpha1.ViolationRecord {
	violationsByPolicy := make(map[string][]v1alpha1.ViolationRecord)
	for nodeName, client := range clients {
		if client == nil {
			r.logger.Info("cannot get a agent client for the node", "node", nodeName)
			continue
		}
		pbViolations, err := client.ScrapeViolations(ctx)
		if err != nil {
			r.agentClientPool.MarkStaleAgentClient(nodeName)
			r.logger.Error(err, "failed to scrape violations", "node", nodeName)
			continue
		}
		for _, v := range pbViolations {
			namespacedName := v.GetPolicyName()
			rec := v1alpha1.ViolationRecord{
				Timestamp:      metav1.NewTime(v.GetTimestamp().AsTime()),
				PodName:        v.GetPodName(),
				ContainerName:  v.GetContainerName(),
				ExecutablePath: v.GetExecutablePath(),
				NodeName:       v.GetNodeName(),
				Action:         v.GetAction(),
				WorkloadName:   v.GetWorkloadName(),
				WorkloadKind:   v.GetWorkloadKind(),
			}
			violationsByPolicy[namespacedName] = append(violationsByPolicy[namespacedName], rec)
		}
	}

	return violationsByPolicy
}

func storeStatusForEachPolicy(
	nodeStatusByPolicy map[string][]v1alpha1.PolicyNodeStatus,
	policies []v1alpha1.WorkloadPolicy,
	nodeStatus v1alpha1.PolicyNodeStatus,
) {
	// Store the node status for the given policy
	for _, policy := range policies {
		policyNamespacedName := policy.NamespacedName()
		nodeStatusByPolicy[policyNamespacedName] = append(
			nodeStatusByPolicy[policyNamespacedName],
			nodeStatus,
		)
	}
}

func (r *WorkloadPolicyStatusSync) getNodeStatusByPolicy(
	ctx context.Context,
	clients map[string]grpcexporter.AgentClientAPI,
	policies []v1alpha1.WorkloadPolicy,
) map[string][]v1alpha1.PolicyNodeStatus {
	nodeStatusByPolicy := make(map[string][]v1alpha1.PolicyNodeStatus, len(policies))
	for _, policy := range policies {
		nodeStatusByPolicy[policy.NamespacedName()] = make([]v1alpha1.PolicyNodeStatus, 0, len(clients))
	}

	for nodeName, client := range clients {
		if client == nil {
			r.logger.Info("cannot get a agent client for the node", "node", nodeName)
			storeStatusForEachPolicy(nodeStatusByPolicy, policies, v1alpha1.PolicyNodeStatus{
				NodeName: nodeName,
				PolicyStatus: v1alpha1.PolicyStatus{
					Code:    v1alpha1.PolicyMissing,
					Message: "No agent client available",
				},
			})
			continue
		}

		nodePolicies, err := client.ListPoliciesStatus(ctx)
		if err != nil {
			r.agentClientPool.MarkStaleAgentClient(nodeName)
			r.logger.Error(err, "failed to get policies status", "node", nodeName)
			storeStatusForEachPolicy(nodeStatusByPolicy, policies, v1alpha1.PolicyNodeStatus{
				NodeName: nodeName,
				PolicyStatus: v1alpha1.PolicyStatus{
					Code:    v1alpha1.PolicyMissing,
					Message: "failed to get policies status",
				},
			})
			continue
		}

		if len(nodePolicies) == 0 {
			r.logger.Error(errors.New("empty policy list"), "No policies found", "node", nodeName)
			storeStatusForEachPolicy(nodeStatusByPolicy, policies, v1alpha1.PolicyNodeStatus{
				NodeName: nodeName,
				PolicyStatus: v1alpha1.PolicyStatus{
					Code:    v1alpha1.PolicyMissing,
					Message: "no policies found on the node",
				},
			})
			continue
		}

		for _, policy := range policies {
			policyNamespacedName := policy.NamespacedName()
			nodeStatus := v1alpha1.PolicyNodeStatus{NodeName: nodeName}

			if nodeStatus.Code, nodeStatus.Message, err = policyNodeStatus(
				policy.Spec.Mode,
				nodePolicies[policyNamespacedName],
			); err != nil {
				r.logger.Error(
					err,
					"failed to get policy node status",
					"node",
					nodeName,
					"policy",
					policyNamespacedName,
				)
				continue
			}

			nodeStatusByPolicy[policyNamespacedName] = append(
				nodeStatusByPolicy[policyNamespacedName],
				nodeStatus,
			)
		}
	}

	return nodeStatusByPolicy
}

func policyNodeStatus(expectedMode string, policyStatus *pb.PolicyStatus) (v1alpha1.PolicyCode, string, error) {
	if policyStatus == nil {
		return v1alpha1.PolicyUnknown, "", errors.New("policy status is nil")
	}

	policyModeMatchesExpected := func(mode pb.PolicyMode, expectedMode string) bool {
		switch expectedMode {
		case policymode.ProtectString:
			return mode == pb.PolicyMode_POLICY_MODE_PROTECT
		case policymode.MonitorString:
			return mode == pb.PolicyMode_POLICY_MODE_MONITOR
		default:
			return false
		}
	}

	switch policyStatus.GetState() {
	case pb.PolicyState_POLICY_STATE_READY:
		if policyModeMatchesExpected(policyStatus.GetMode(), expectedMode) {
			return v1alpha1.PolicyReady, "", nil
		}
		return v1alpha1.PolicyTransitioning, "", nil
	case pb.PolicyState_POLICY_STATE_ERROR:
		msg := policyStatus.GetMessage()
		if msg == "" {
			msg = "policy is in error state"
		}
		return v1alpha1.PolicyFailed, msg, nil
	case pb.PolicyState_POLICY_STATE_UNSPECIFIED:
		fallthrough
	default:
		return v1alpha1.PolicyUnknown, "", fmt.Errorf("unknown policy state %q",
			policyStatus.GetState().String())
	}
}

// processWorkloadPolicy updates the wp.status and wp.annotation in order to acknowledge a violation.
// NOTE: agent side ignores annotation changes and status change via predicate.GenerationChangedPredicate{}.
func (r *WorkloadPolicyStatusSync) processWorkloadPolicy(
	ctx context.Context,
	wp *v1alpha1.WorkloadPolicy,
	nodeStatuses []v1alpha1.PolicyNodeStatus,
	scrapedViolations []v1alpha1.ViolationRecord,
) error {
	patchBase := client.MergeFrom(wp.DeepCopy())
	newPolicy := wp.DeepCopy()

	if err := newPolicy.ProcessPolicyStatus(nodeStatuses, scrapedViolations, metav1.NewTime(time.Now())); err != nil {
		return fmt.Errorf("failed to compute status for policy %s: %w", wp.NamespacedName(), err)
	}

	oldAckIDs := make(map[int64]struct{}, len(wp.Status.AcknowledgedViolations))
	for _, ack := range wp.Status.AcknowledgedViolations {
		oldAckIDs[ack.Violation.ID] = struct{}{}
	}

	for _, ack := range newPolicy.Status.AcknowledgedViolations {
		if _, exists := oldAckIDs[ack.Violation.ID]; exists {
			continue
		}
		r.emitAcknowledgedViolationOtelLog(ctx, ack)
	}

	r.logger.V(loglevel.VerbosityDebug).Info("updating",
		"policy", newPolicy.NamespacedName(),
		"annotations", newPolicy.Annotations,
		"status", newPolicy.Status)

	// At this point, we already have the expected WorkloadPolicy.
	// Due to kubernetes design, we have to call update annotations and status separately.
	// Here we use Patch() to prevent annotation changes made between two calls from being lost.

	// We update status first and remove the annotations later
	// If anything goes wrong we can retry in the next reconcile.
	err := r.Status().Patch(ctx, newPolicy.DeepCopy(), patchBase)
	if err != nil {
		return err
	}

	err = r.Patch(ctx, newPolicy.DeepCopy(), patchBase)
	if err != nil {
		return err
	}
	return nil
}

func (r *WorkloadPolicyStatusSync) emitAcknowledgedViolationOtelLog(
	ctx context.Context,
	ack v1alpha1.AcknowledgedViolationRecord,
) {
	if r.eventLogger == nil {
		return
	}
	violation := ack.Violation
	var rec otellog.Record
	rec.SetEventName("policy_violation_acknowledged")
	rec.SetSeverity(otellog.SeverityInfo)
	rec.SetBody(otellog.StringValue("policy_violation_acknowledged"))
	rec.SetTimestamp(time.Now())
	rec.AddAttributes(
		otellog.Int64("id", violation.ID),
		otellog.String("timestamp", violation.Timestamp.UTC().Format(time.RFC3339)),
		otellog.String("reason", ack.Reason),
		otellog.String("k8s.pod.name", violation.PodName),
		otellog.String("container.name", violation.ContainerName),
		otellog.String("proc.exepath", violation.ExecutablePath),
		otellog.String("node.name", violation.NodeName),
		otellog.String("action", violation.Action),
		otellog.String("workload.name", violation.WorkloadName),
		otellog.String("workload.kind", violation.WorkloadKind),
	)

	r.eventLogger.Emit(ctx, rec)
}
