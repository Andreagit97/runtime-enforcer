package e2e_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/rancher-sandbox/runtime-enforcer/api/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerywait "k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
)

func getResources(ctx context.Context) *resources.Resources {
	return ctx.Value(key("client")).(*resources.Resources)
}

func createTestNamespace(ctx context.Context, t *testing.T, namespace string) {
	t.Helper()
	t.Logf("creating test namespace: %q", namespace)
	err := getResources(ctx).Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	require.NoError(t, err, "failed to create test namespace %q", namespace)
}

////////////////////
// Workload Policy helpers
////////////////////

func createAndWaitWP(ctx context.Context, t *testing.T, policy *v1alpha1.WorkloadPolicy) {
	t.Helper()
	t.Logf("creating workload policy %q and waiting for it to become Active", policy.NamespacedName())
	err := getResources(ctx).Create(ctx, policy)
	require.NoError(t, err, "failed to create workload policy %q", policy.NamespacedName())
	waitForWorkloadPolicyStatusToBeUpdated(ctx, t, policy)
}

func deleteAndWaitWP(ctx context.Context, t *testing.T, policy *v1alpha1.WorkloadPolicy) {
	t.Helper()
	t.Logf("deleting workload policy %q and waiting for it to be deleted", policy.NamespacedName())
	err := getResources(ctx).Delete(ctx, policy)
	require.NoError(t, err, "failed to delete workload policy %q", policy.NamespacedName())
	err = wait.For(
		conditions.New(getResources(ctx)).ResourceDeleted(policy),
		wait.WithTimeout(DefaultOperationTimeout),
	)
	require.NoError(t, err, "workload policy %q cannot be deleted", policy.NamespacedName())
}

func waitForWorkloadPolicyStatusToBeUpdated(
	ctx context.Context,
	t *testing.T,
	policy *v1alpha1.WorkloadPolicy,
) {
	r := ctx.Value(key("client")).(*resources.Resources)
	err := wait.For(conditions.New(r).ResourceMatch(policy, func(obj k8s.Object) bool {
		ps, ok := obj.(*v1alpha1.WorkloadPolicy)
		if !ok {
			return false
		}
		t.Log("checking workloadpolicy status:", ps.Status)
		if ps.Status.ObservedGeneration != ps.Generation {
			return false
		}
		if ps.Status.Phase != v1alpha1.Active {
			return false
		}
		if len(ps.Status.NodesTransitioning) != 0 {
			return false
		}
		if len(ps.Status.NodesWithIssues) != 0 {
			return false
		}
		return true
	}), wait.WithTimeout(60*time.Second))
	require.NoError(t, err, "workloadpolicy status should be updated to Deployed")
}

func verifyUbuntuLearnedProcesses(values []string) bool {
	return slices.Contains(values, "/usr/bin/bash") &&
		slices.Contains(values, "/usr/bin/ls") &&
		slices.Contains(values, "/usr/bin/sleep")
}

func getDeploymentPolicyMutateOption(
	namespace string,
	policy string, //nolint:unparam // we want to keep the flexibility to support different policy name.
) decoder.DecodeOption {
	// Support only deployment right now.
	return decoder.MutateOption(func(obj k8s.Object) error {
		deployment := obj.(*appsv1.Deployment)
		deployment.SetNamespace(namespace)
		deployment.Spec.Template.Labels[v1alpha1.PolicyLabelKey] = policy
		return nil
	})
}

func daemonSetUpToDate(r *resources.Resources, daemonset *appsv1.DaemonSet) apimachinerywait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		if err := r.Get(ctx, daemonset.GetName(), daemonset.GetNamespace(), daemonset); err != nil {
			return false, err
		}
		status := daemonset.Status
		if status.UpdatedNumberScheduled != status.DesiredNumberScheduled {
			return false, nil
		}
		return true, nil
	}
}

func deploymentUpToDate(
	r *resources.Resources,
	deployment *appsv1.Deployment,
) apimachinerywait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		if err := r.Get(ctx, deployment.GetName(), deployment.GetNamespace(), deployment); err != nil {
			return false, err
		}
		status := deployment.Status
		if status.Replicas != *deployment.Spec.Replicas {
			return false, nil
		}
		if status.UpdatedReplicas != status.Replicas {
			return false, nil
		}
		return true, nil
	}
}
