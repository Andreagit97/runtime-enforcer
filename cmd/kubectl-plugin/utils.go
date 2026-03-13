package main

import (
	"context"
	"fmt"
	"time"

	securityclient "github.com/rancher-sandbox/runtime-enforcer/pkg/generated/clientset/versioned/typed/api/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

// subcommandUsageTemplate is a custom usage template for subcommands:
// it does not print the "Available Commands" section.
const subcommandUsageTemplate = `Usage:
  {{.UseLine}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
`

const (
	defaultOperationTimeout = 30 * time.Second
	defaultPollInterval     = 500 * time.Millisecond
)

type commonOptions struct {
	configFlags *genericclioptions.ConfigFlags
	ioStreams   genericclioptions.IOStreams

	Namespace     string
	AllNamespaces bool
	DryRun        bool
}

func (o *commonOptions) AddCommonFlags(cmd *cobra.Command) {
	o.configFlags.AddFlags(cmd.Flags())
	cmd.Flags().BoolVarP(&o.AllNamespaces, "all-namespaces", "A", false, "Se presente, agisce su tutti i namespace")
	cmd.Flags().BoolVar(&o.DryRun, "dry-run", false, "Simula l'operazione senza salvare")
}

type subcommandFunc func(
	ctx context.Context,
	securityClient securityclient.SecurityV1alpha1Interface,
	namespace string,
) error

// withRuntimeEnforcerClient is a helper function to create a runtime-enforcer client and execute a subcommand.
// It uses ConfigFlags (from the root command's persistent flags) to resolve kubeconfig, context and namespace.
func withRuntimeEnforcerClient(
	cmd *cobra.Command,
	configFlags *genericclioptions.ConfigFlags,
	subcommand subcommandFunc,
) error {
	config, err := configFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("failed to load Kubernetes configuration: %w", err)
	}

	namespace, _, err := configFlags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return fmt.Errorf("failed to determine namespace: %w", err)
	}

	securityClient, err := securityclient.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create runtime-enforcer client: %w", err)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), defaultOperationTimeout)
	defer cancel()

	return subcommand(ctx, securityClient, namespace)
}

// configFlagsFromCmd retrieves the ConfigFlags bound to the root command's persistent flags.
func configFlagsFromCmd(cmd *cobra.Command) *genericclioptions.ConfigFlags {
	cf := genericclioptions.NewConfigFlags(true)
	cf.AddFlags(cmd.Root().PersistentFlags())
	return cf
}

// ioStreams returns an IOStreams backed by the cobra command's standard streams.
func ioStreams(cmd *cobra.Command) genericiooptions.IOStreams {
	return genericiooptions.IOStreams{
		In:     cmd.InOrStdin(),
		Out:    cmd.OutOrStdout(),
		ErrOut: cmd.ErrOrStderr(),
	}
}
