package v1alpha1_test

import (
	"testing"

	"github.com/rancher-sandbox/runtime-enforcer/api/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkloadPolicyNamespacedName(t *testing.T) {
	wp := &v1alpha1.WorkloadPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-name",
		},
	}
	expected := "test-namespace/test-name"
	require.Equal(t, expected, wp.NamespacedName())
}

func TestProcessNodeStatusCounters(t *testing.T) {
	wp := &v1alpha1.WorkloadPolicy{
		Status: v1alpha1.WorkloadPolicyStatus{},
	}

	err := wp.ProcessPolicyStatus(
		[]v1alpha1.PolicyNodeStatus{{NodeName: "n1", PolicyStatus: v1alpha1.PolicyStatus{Code: v1alpha1.PolicyReady}}, {
			NodeName: "n2",
			PolicyStatus: v1alpha1.PolicyStatus{
				Code: v1alpha1.PolicyTransitioning,
			},
		}, {
			NodeName: "n3",
			PolicyStatus: v1alpha1.PolicyStatus{
				Code:    v1alpha1.PolicyFailed,
				Message: "boom",
			},
		}},
		nil,
		metav1.Now(),
	)
	require.NoError(t, err)

	require.Equal(t, 3, wp.Status.TotalNodes)
	require.Equal(t, 1, wp.Status.SuccessfulNodes)
	require.Equal(t, 1, wp.Status.TransitioningNodes)
	require.Equal(t, 1, wp.Status.FailedNodes)
	require.Equal(t, v1alpha1.Failed, wp.Status.Phase)
	require.Equal(t, []string{"n2"}, wp.Status.NodesTransitioning)
	require.Equal(
		t,
		map[string]v1alpha1.PolicyStatus{"n3": {Code: v1alpha1.PolicyFailed, Message: "boom"}},
		wp.Status.NodesWithIssues,
	)
}
