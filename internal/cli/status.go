package cli

import (
	"encoding/json"
	"github.com/arjav1528/Tracelet/internal/config"
	"github.com/spf13/cobra"
)

func statusCommand() *cobra.Command {
	var configPath string
	command := &cobra.Command{Use: "status", Short: "Report the selected project's session without starting services", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			status := struct {
				Active           bool   `json:"active"`
				ProjectID        string `json:"project_id"`
				ProjectName      string `json:"project_name"`
				Role             string `json:"role,omitempty"`
				UIURL            string `json:"ui_url,omitempty"`
				ReceiverEndpoint string `json:"receiver_endpoint,omitempty"`
				Message          string `json:"message"`
			}{ProjectID: cfg.Project.ID, ProjectName: cfg.Project.Name, Message: "No verified active session. Use tracelet ui for stored history or tracelet run to capture traces."}
			if d, err := discover(command.Context(), cfg); err == nil {
				status.Active = true
				status.Role = d.Role
				status.UIURL = d.UIURL
				status.ReceiverEndpoint = d.ReceiverEndpoint
				status.Message = "Verified local session."
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(status)
		}}
	command.Flags().StringVar(&configPath, "config", ".tracelet/config.yaml", "Project configuration path")
	return command
}
