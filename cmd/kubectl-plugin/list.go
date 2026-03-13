package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	apiv1alpha1 "github.com/rancher-sandbox/runtime-enforcer/api/v1alpha1"
	securityclient "github.com/rancher-sandbox/runtime-enforcer/pkg/generated/clientset/versioned/typed/api/v1alpha1"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/kubernetes"
)

type listOptions struct {
	AllNamespaces bool
	OutputFormat  string
}

// listEntry represents a single row in the list output.
type listEntry struct {
	Namespace  string `json:"namespace"`
	PolicyName string `json:"policyName"`
	Mode       string `json:"mode"`
	PodName    string `json:"podName"`
}

func newListCmd(configFlags *genericclioptions.ConfigFlags) *cobra.Command {
	opts := &listOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workloads and the policies protecting them",
		Long:  "List which workloads are protected by which WorkloadPolicy.",
		Args:  cobra.NoArgs,
		RunE:  runListCmd(configFlags, opts),
	}

	cmd.SetUsageTemplate(subcommandUsageTemplate)

	cmd.Flags().BoolVarP(&opts.AllNamespaces, "all-namespaces", "A", false, "List workloads across all namespaces")
	cmd.Flags().StringVarP(&opts.OutputFormat, "output", "o", "table", "Output format: table or json")

	return cmd
}

func runListCmd(configFlags *genericclioptions.ConfigFlags, opts *listOptions) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		if opts.OutputFormat != "table" && opts.OutputFormat != "json" {
			return fmt.Errorf("invalid output format %q, expected \"table\" or \"json\"", opts.OutputFormat)
		}

		config, err := configFlags.ToRESTConfig()
		if err != nil {
			return fmt.Errorf("failed to load Kubernetes configuration: %w", err)
		}

		namespace, _, err := configFlags.ToRawKubeConfigLoader().Namespace()
		if err != nil {
			return fmt.Errorf("failed to determine namespace: %w", err)
		}

		if opts.AllNamespaces {
			namespace = ""
		}

		securityClient, err := securityclient.NewForConfig(config)
		if err != nil {
			return fmt.Errorf("failed to create runtime-enforcer client: %w", err)
		}

		k8sClient, err := kubernetes.NewForConfig(config)
		if err != nil {
			return fmt.Errorf("failed to create Kubernetes client: %w", err)
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), defaultOperationTimeout)
		defer cancel()

		streams := ioStreams(cmd)
		return runList(ctx, securityClient, k8sClient, namespace, opts, streams.Out)
	}
}

func runList(
ctx context.Context,
securityClient securityclient.SecurityV1alpha1Interface,
k8sClient kubernetes.Interface,
namespace string,
opts *listOptions,
out io.Writer,
) error {
	policies, err := securityClient.WorkloadPolicies(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list WorkloadPolicies: %w", err)
	}

	entries, err := buildListEntries(ctx, k8sClient, policies.Items)
	if err != nil {
		return err
	}

	switch opts.OutputFormat {
	case "json":
		return printListJSON(out, entries)
	default:
		return printListTable(out, entries)
	}
}

func buildListEntries(
ctx context.Context,
k8sClient kubernetes.Interface,
policies []apiv1alpha1.WorkloadPolicy,
) ([]listEntry, error) {
	var entries []listEntry

	for i := range policies {
		policy := &policies[i]

		pods, err := k8sClient.CoreV1().Pods(policy.Namespace).List(ctx, metav1.ListOptions{
LabelSelector: apiv1alpha1.PolicyLabelKey + "=" + policy.Name,
})
		if err != nil {
			return nil, fmt.Errorf(
"failed to list pods for WorkloadPolicy %q in namespace %q: %w",
policy.Name,
policy.Namespace,
err,
)
		}

		if len(pods.Items) == 0 {
			// Include the policy even if there are no pods yet.
			entries = append(entries, listEntry{
Namespace:  policy.Namespace,
PolicyName: policy.Name,
Mode:       policy.Spec.Mode,
PodName:    "",
})
			continue
		}

		for j := range pods.Items {
			entries = append(entries, listEntry{
Namespace:  policy.Namespace,
PolicyName: policy.Name,
Mode:       policy.Spec.Mode,
PodName:    pods.Items[j].Name,
})
		}
	}

	return entries, nil
}

func printListTable(out io.Writer, entries []listEntry) error {
	w := printers.GetNewTabWriter(out)
	fmt.Fprintln(w, "NAMESPACE\tPOLICY\tMODE\tPOD")

	for _, e := range entries {
		podName := e.PodName
		if podName == "" {
			podName = "<none>"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Namespace, e.PolicyName, e.Mode, podName)
	}

	return w.Flush()
}

func printListJSON(out io.Writer, entries []listEntry) error {
	if entries == nil {
		entries = []listEntry{}
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}
