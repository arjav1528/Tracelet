package cli

import "github.com/spf13/cobra"

func NewRoot(version, commit string) *cobra.Command {
	command := &cobra.Command{
		Use:           "tracelet",
		Short:         "Inspect OpenTelemetry traces locally",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	command.AddCommand(versionCommand(version, commit), initCommand(), runCommand(), uiCommand(openBrowser), statusCommand())
	return command
}
