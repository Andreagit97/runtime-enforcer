//nolint:testpackage // we are testing unexported functions
package resolver

import (
	"log/slog"
	"testing"

	"github.com/rancher-sandbox/runtime-enforcer/api/v1alpha1"
	"github.com/rancher-sandbox/runtime-enforcer/internal/bpf"
	"github.com/rancher-sandbox/runtime-enforcer/internal/types/policymode"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func mockPolicyUpdateBinariesFunc(_ PolicyID, _ []string, _ bpf.PolicyValuesOperation) error {
	return nil
}
func mockPolicyModeUpdateFunc(_ PolicyID, _ policymode.Mode, _ bpf.PolicyModeOperation) error {
	return nil
}
func mockCgTrackerUpdateFunc(_ uint64, _ string) error { return nil }
func mockCgroupToPolicyMapUpdateFunc(_ PolicyID, _ []CgroupID, _ bpf.CgroupPolicyOperation) error {
	return nil
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Helper()
	w.t.Log(string(p))
	return len(p), nil
}

func TestResolver(t *testing.T) {
	// Your test code here
	res, err := NewResolver(
		slog.New(slog.NewTextHandler(testWriter{t}, nil)),
		mockCgTrackerUpdateFunc,
		mockCgroupToPolicyMapUpdateFunc,
		mockPolicyUpdateBinariesFunc,
		mockPolicyModeUpdateFunc,
	)
	require.NoError(t, err)

	container1 := "container1"
	container2 := "container2"
	container3 := "container3"

	wp := v1alpha1.WorkloadPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test-namespace",
		},
		Spec: v1alpha1.WorkloadPolicySpec{
			Mode: "monitor",
			RulesByContainer: map[string]*v1alpha1.WorkloadPolicyRules{
				container1: {
					Executables: v1alpha1.WorkloadPolicyExecutables{
						Allowed: []string{"/usr/bin/sleep"},
					},
				},
				container2: {
					Executables: v1alpha1.WorkloadPolicyExecutables{
						Allowed: []string{"/usr/bin/cat"},
					},
				},
			},
		},
	}

	// First add a new policy
	require.NoError(t, res.handleWPAdd(&wp))
	require.Contains(t, res.wpState, wp.NamespacedName())
	require.Equal(t, map[string]PolicyID{
		container1: PolicyID(1),
		container2: PolicyID(2),
	}, res.wpState[wp.NamespacedName()])

	// Now we update the policy
	// we delete the container1
	delete(wp.Spec.RulesByContainer, container1)
	// we add a new container3
	wp.Spec.RulesByContainer[container3] = &v1alpha1.WorkloadPolicyRules{
		Executables: v1alpha1.WorkloadPolicyExecutables{
			Allowed: []string{"/usr/bin/ls"},
		},
	}

	// we handle the update
	require.NoError(t, res.handleWPUpdate(&wp))
	require.Contains(t, res.wpState, wp.NamespacedName())
	require.Equal(t, map[string]PolicyID{
		container2: PolicyID(2),
		container3: PolicyID(3),
	}, res.wpState[wp.NamespacedName()])
}
