package main

import (
"context"
"encoding/json"
"strings"
"testing"

apiv1alpha1 "github.com/rancher-sandbox/runtime-enforcer/api/v1alpha1"
fakesecurityclient "github.com/rancher-sandbox/runtime-enforcer/pkg/generated/clientset/versioned/fake"
"github.com/stretchr/testify/require"
corev1 "k8s.io/api/core/v1"
metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
"k8s.io/cli-runtime/pkg/genericiooptions"
kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

func TestRunListTableWithPods(t *testing.T) {
	t.Parallel()

	ns := "test"
	policyName := "my-policy"

	policy := &apiv1alpha1.WorkloadPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: ns,
		},
		Spec: apiv1alpha1.WorkloadPolicySpec{
			Mode: "protect",
		},
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: ns,
			Labels: map[string]string{
				apiv1alpha1.PolicyLabelKey: policyName,
			},
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: ns,
			Labels: map[string]string{
				apiv1alpha1.PolicyLabelKey: policyName,
			},
		},
	}

	clientset := fakesecurityclient.NewClientset(policy)
	securityClient := clientset.SecurityV1alpha1()
	k8sClient := kubernetesfake.NewClientset(pod1, pod2)

	streams, _, out, _ := genericiooptions.NewTestIOStreams()
	opts := &listOptions{
		OutputFormat: "table",
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer cancel()

	err := runList(ctx, securityClient, k8sClient, ns, opts, streams.Out)
	require.NoError(t, err)

	output := out.String()
	require.Contains(t, output, "NAMESPACE")
	require.Contains(t, output, "POLICY")
	require.Contains(t, output, "MODE")
	require.Contains(t, output, "POD")
	require.Contains(t, output, policyName)
	require.Contains(t, output, "protect")
	require.Contains(t, output, "pod-1")
	require.Contains(t, output, "pod-2")
}

func TestRunListTableNoPods(t *testing.T) {
	t.Parallel()

	ns := "test"
	policyName := "my-policy"

	policy := &apiv1alpha1.WorkloadPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: ns,
		},
		Spec: apiv1alpha1.WorkloadPolicySpec{
			Mode: "monitor",
		},
	}

	clientset := fakesecurityclient.NewClientset(policy)
	securityClient := clientset.SecurityV1alpha1()
	k8sClient := kubernetesfake.NewClientset()

	streams, _, out, _ := genericiooptions.NewTestIOStreams()
	opts := &listOptions{
		OutputFormat: "table",
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer cancel()

	err := runList(ctx, securityClient, k8sClient, ns, opts, streams.Out)
	require.NoError(t, err)

	output := out.String()
	require.Contains(t, output, policyName)
	require.Contains(t, output, "monitor")
	require.Contains(t, output, "<none>")
}

func TestRunListNoPolicies(t *testing.T) {
	t.Parallel()

	ns := "test"

	clientset := fakesecurityclient.NewClientset()
	securityClient := clientset.SecurityV1alpha1()
	k8sClient := kubernetesfake.NewClientset()

	streams, _, out, _ := genericiooptions.NewTestIOStreams()
	opts := &listOptions{
		OutputFormat: "table",
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer cancel()

	err := runList(ctx, securityClient, k8sClient, ns, opts, streams.Out)
	require.NoError(t, err)

	output := out.String()
	// Only the header row should be present.
	require.Contains(t, output, "NAMESPACE")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 1, "expected only the header line when no policies exist")
}

func TestRunListJSONOutput(t *testing.T) {
	t.Parallel()

	ns := "test"
	policyName := "my-policy"

	policy := &apiv1alpha1.WorkloadPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: ns,
		},
		Spec: apiv1alpha1.WorkloadPolicySpec{
			Mode: "protect",
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: ns,
			Labels: map[string]string{
				apiv1alpha1.PolicyLabelKey: policyName,
			},
		},
	}

	clientset := fakesecurityclient.NewClientset(policy)
	securityClient := clientset.SecurityV1alpha1()
	k8sClient := kubernetesfake.NewClientset(pod)

	streams, _, out, _ := genericiooptions.NewTestIOStreams()
	opts := &listOptions{
		OutputFormat: "json",
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer cancel()

	err := runList(ctx, securityClient, k8sClient, ns, opts, streams.Out)
	require.NoError(t, err)

	var entries []listEntry
	require.NoError(t, json.Unmarshal(out.Bytes(), &entries))
	require.Len(t, entries, 1)
	require.Equal(t, ns, entries[0].Namespace)
	require.Equal(t, policyName, entries[0].PolicyName)
	require.Equal(t, "protect", entries[0].Mode)
	require.Equal(t, "pod-1", entries[0].PodName)
}

func TestRunListJSONOutputNoPolicies(t *testing.T) {
	t.Parallel()

	ns := "test"

	clientset := fakesecurityclient.NewClientset()
	securityClient := clientset.SecurityV1alpha1()
	k8sClient := kubernetesfake.NewClientset()

	streams, _, out, _ := genericiooptions.NewTestIOStreams()
	opts := &listOptions{
		OutputFormat: "json",
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer cancel()

	err := runList(ctx, securityClient, k8sClient, ns, opts, streams.Out)
	require.NoError(t, err)

	var entries []listEntry
	require.NoError(t, json.Unmarshal(out.Bytes(), &entries))
	require.Empty(t, entries)
}

func TestRunListMultiplePoliciesAndNamespaces(t *testing.T) {
	t.Parallel()

	ns1 := "ns-1"
	ns2 := "ns-2"

	policy1 := &apiv1alpha1.WorkloadPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-a", Namespace: ns1},
		Spec:       apiv1alpha1.WorkloadPolicySpec{Mode: "monitor"},
	}
	policy2 := &apiv1alpha1.WorkloadPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "policy-b", Namespace: ns2},
		Spec:       apiv1alpha1.WorkloadPolicySpec{Mode: "protect"},
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-a",
			Namespace: ns1,
			Labels:    map[string]string{apiv1alpha1.PolicyLabelKey: "policy-a"},
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-b",
			Namespace: ns2,
			Labels:    map[string]string{apiv1alpha1.PolicyLabelKey: "policy-b"},
		},
	}

	// All namespaces: pass "" as namespace.
	clientset := fakesecurityclient.NewClientset(policy1, policy2)
	securityClient := clientset.SecurityV1alpha1()
	k8sClient := kubernetesfake.NewClientset(pod1, pod2)

	streams, _, out, _ := genericiooptions.NewTestIOStreams()
	opts := &listOptions{
		AllNamespaces: true,
		OutputFormat:  "json",
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultOperationTimeout)
	defer cancel()

	err := runList(ctx, securityClient, k8sClient, "" /* all namespaces */, opts, streams.Out)
	require.NoError(t, err)

	var entries []listEntry
	require.NoError(t, json.Unmarshal(out.Bytes(), &entries))
	require.Len(t, entries, 2)

	byPolicy := make(map[string]listEntry)
	for _, e := range entries {
		byPolicy[e.PolicyName] = e
	}

	require.Equal(t, "monitor", byPolicy["policy-a"].Mode)
	require.Equal(t, "pod-a", byPolicy["policy-a"].PodName)
	require.Equal(t, "protect", byPolicy["policy-b"].Mode)
	require.Equal(t, "pod-b", byPolicy["policy-b"].PodName)
}
