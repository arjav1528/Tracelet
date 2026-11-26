package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func versionCommand(version, commit string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit and telemetry protocol",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(command.OutOrStdout(), "Tracelet %s\ncommit: %s\nprotocol: otlp-http-protobuf\n", version, commit)
			return err
		},
	}
}
