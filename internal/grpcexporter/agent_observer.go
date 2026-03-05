package grpcexporter

import (
	"context"

	"log/slog"

	"github.com/rancher-sandbox/runtime-enforcer/internal/resolver"
	pb "github.com/rancher-sandbox/runtime-enforcer/proto/agent/v1"
)

// agentObserver implements the AgentObserver gRPC server.
type agentObserver struct {
	pb.UnimplementedAgentObserverServer

	logger   *slog.Logger
	resolver *resolver.Resolver
}

func newAgentObserver(logger *slog.Logger, resolver *resolver.Resolver) *agentObserver {
	return &agentObserver{
		logger:   logger.With("component", "agent_observer"),
		resolver: resolver,
	}
}

// ListPoliciesStatus list policies inside the resolver and returns their status.
func (s *agentObserver) ListPoliciesStatus(
	ctx context.Context,
	_ *pb.ListPoliciesStatusRequest,
) (*pb.ListPoliciesStatusResponse, error) {
	out := &pb.ListPoliciesStatusResponse{
		Policies: make(map[string]*pb.PolicyStatus),
	}

	statuses := s.resolver.GetPolicyStatuses()
	for policyName, ps := range statuses {
		out.Policies[policyName] = &pb.PolicyStatus{
			State:   ps.State,
			Mode:    ps.Mode,
			Message: ps.Message,
		}
	}

	s.logger.DebugContext(ctx, "listed tracing policies", "count", len(out.GetPolicies()))
	return out, nil
}

func podViewToProto(podView *resolver.PodView) *pb.PodView {
	view := &pb.PodView{
		Meta: &pb.PodMeta{
			ID:           podView.Meta.ID,
			Name:         podView.Meta.Name,
			Namespace:    podView.Meta.Namespace,
			WorkloadName: podView.Meta.WorkloadName,
			WorkloadType: podView.Meta.WorkloadType,
			Labels:       podView.Meta.Labels,
		},
		Containers: make(map[string]*pb.ContainerMeta, len(podView.Containers)),
	}
	for containerID, containerMeta := range podView.Containers {
		view.Containers[containerID] = &pb.ContainerMeta{
			ID:       containerID,
			Name:     containerMeta.Name,
			CgroupID: containerMeta.CgroupID,
		}
	}
	return view
}

// ListPodCache lists the current pod cache.
func (s *agentObserver) ListPodCache(
	ctx context.Context,
	_ *pb.ListPodCacheRequest,
) (*pb.ListPodCacheResponse, error) {
	out := &pb.ListPodCacheResponse{
		Pods: []*pb.PodView{},
	}

	snapshot := s.resolver.PodCacheSnapshot()
	for _, podView := range snapshot {
		out.Pods = append(out.Pods, podViewToProto(&podView))
	}

	s.logger.DebugContext(ctx, "listed pod cache", "count", len(out.GetPods()))
	return out, nil
}
