package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"tracelet.local/tracelet/internal/collector"
	"tracelet.local/tracelet/internal/config"
	"tracelet.local/tracelet/internal/storage"
	"tracelet.local/tracelet/internal/supervisor"
)

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("application exited with status %d: %v", e.Code, e.Err)
	}
	return fmt.Sprintf("application exited with status %d", e.Code)
}
func (e *ExitError) Unwrap() error { return e.Err }

func runCommand() *cobra.Command {
	var configPath string
	var override bool
	command := &cobra.Command{
		Use: "run [flags] -- command [args...]", Short: "Run an already-instrumented application with local trace capture", Args: cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) (result error) {
			if command.ArgsLenAtDash() < 0 {
				return errors.New("put -- before the application command")
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			lock, err := acquireProject(command.Context(), cfg)
			if err != nil {
				return err
			}
			defer func() { result = errors.Join(result, lock.Close()) }()
			store, err := storage.Open(command.Context(), cfg)
			if err != nil {
				return err
			}
			defer func() { result = errors.Join(result, store.Close()) }()
			stopMaintenance, err := startMaintenance(command.Context(), store, command.ErrOrStderr())
			if err != nil {
				return err
			}
			defer func() { result = errors.Join(result, stopMaintenance()) }()
			receiver, err := collector.Start(cfg, store)
			if err != nil {
				return err
			}
			grace := time.Duration(cfg.ShutdownGraceSeconds) * time.Second
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), grace)
				defer cancel()
				result = errors.Join(result, receiver.Close(ctx))
			}()
			ui, descriptor, err := startUI(command.Context(), cfg, "run", receiver.Endpoint())
			if err != nil {
				return err
			}
			defer func() { result = errors.Join(result, closeUI(cfg, ui, descriptor)) }()
			if _, err := fmt.Fprintln(command.ErrOrStderr(), "Tracelet UI:", ui.URL(), "(open with tracelet ui)"); err != nil {
				return err
			}
			values, notices := supervisor.Env(os.Environ(), receiver.Endpoint(), cfg.Project.Name, override)
			if _, err := fmt.Fprintln(command.ErrOrStderr(), "Tracelet receiver:", receiver.Endpoint()); err != nil {
				return err
			}
			for _, notice := range notices {
				if _, err := fmt.Fprintln(command.ErrOrStderr(), notice); err != nil {
					return err
				}
			}
			code, err := supervisor.Run(command.Context(), args, values, command.OutOrStdout(), command.ErrOrStderr(), grace)
			if code != 0 {
				return &ExitError{Code: code, Err: err}
			}
			return err
		},
	}
	command.Flags().StringVar(&configPath, "config", ".tracelet/config.yaml", "Project configuration path")
	command.Flags().BoolVar(&override, "override-otel", false, "Explicitly redirect the child trace exporter to Tracelet")
	return command
}
