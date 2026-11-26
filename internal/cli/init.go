package cli

import (
	"fmt"
	"os"

	"github.com/arjav1528/Tracelet/internal/config"
	"github.com/spf13/cobra"
)

func initCommand() *cobra.Command {
	var overwrite bool
	command := &cobra.Command{
		Use: "init", Short: "Create local project configuration", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			path, err := config.Init(root, overwrite)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "Initialized %s\nTracelet requires OpenTelemetry instrumentation in your application.\n", path)
			return err
		},
	}
	command.Flags().BoolVar(&overwrite, "force", false, "Replace existing configuration with defaults and a new project identity")
	return command
}
