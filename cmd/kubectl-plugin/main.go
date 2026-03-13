package main

import (
	"os"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

var version = "dev"

// Custom usage template: no "kubectl [command]" line.
const (
	rootUsageTemplate = `Usage:
  {{.UseLine}}

Available Commands:
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}  {{rpad .Name .NamePadding}} {{.Short}}
{{end}}{{end}}
Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}
`
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "kubectl runtime-enforcer",
		Long:    "Kubernetes plugin for SUSE Security Runtime Enforcer",
		Version: version,
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	// Adds --kubeconfig, --context, -n/--namespace, --server, --token, etc.
	configFlags := genericclioptions.NewConfigFlags(true)
	configFlags.AddFlags(cmd.PersistentFlags())

	cmd.SetUsageTemplate(rootUsageTemplate)

	cmd.AddCommand(newMarkReadyCmd(configFlags))
	cmd.AddCommand(newSwitchModeCmd(configFlags))
	cmd.AddCommand(newListCmd(configFlags))

	return cmd
}

func main() {
	streams := genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
