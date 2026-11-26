package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"tracelet.local/tracelet/internal/config"
	"tracelet.local/tracelet/internal/server"
	"tracelet.local/tracelet/internal/session"
	"tracelet.local/tracelet/internal/storage"
)

func descriptorPath(cfg config.Config) string { return cfg.Storage.Path + ".session.json" }

func discover(ctx context.Context, cfg config.Config) (session.Descriptor, error) {
	d, err := session.Read(descriptorPath(cfg))
	if err != nil {
		return d, err
	}
	if d.ProjectID != cfg.Project.ID {
		return session.Descriptor{}, errors.New("session belongs to another project")
	}
	return d, session.Probe(ctx, d)
}

func acquireProject(ctx context.Context, cfg config.Config) (*session.Lock, error) {
	lock, err := session.Acquire(cfg.Storage.Path + ".lock")
	if errors.Is(err, session.ErrOwned) {
		if d, probeErr := discover(ctx, cfg); probeErr == nil && d.Role == "history" {
			return nil, errors.New("project history UI is active; stop it with Ctrl+C before running an application")
		}
		return nil, errors.New("another Tracelet process owns this project; use tracelet status or stop that process first")
	}
	return lock, err
}

func startUI(ctx context.Context, cfg config.Config, role, endpoint string) (*server.Server, session.Descriptor, error) {
	d, err := session.NewDescriptor(cfg.Project.ID, role)
	if err != nil {
		return nil, d, err
	}
	d.ReceiverEndpoint = endpoint
	ui, err := server.Start(cfg, d)
	if err != nil {
		return nil, d, err
	}
	d.UIURL = ui.URL()
	if err = session.Probe(ctx, d); err == nil {
		err = session.Publish(descriptorPath(cfg), d)
	}
	if err != nil {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		return nil, d, errors.Join(err, ui.Close(shutdown))
	}
	return ui, d, nil
}

func closeUI(cfg config.Config, ui *server.Server, d session.Descriptor) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.ShutdownGraceSeconds)*time.Second)
	defer cancel()
	return errors.Join(session.Remove(descriptorPath(cfg), d.InstanceID), ui.Close(ctx))
}

func uiCommand(open func(string) error) *cobra.Command {
	var configPath string
	var noOpen bool
	command := &cobra.Command{Use: "ui", Short: "Open the active UI or serve stored history until Ctrl+C", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) (result error) {
			ctx, stop := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			command.SetContext(ctx)
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			present := func(d session.Descriptor) error {
				address := d.UIURL + "/#token=" + d.Nonce
				if _, err := fmt.Fprintln(command.OutOrStdout(), address); err != nil {
					return err
				}
				if !noOpen && cfg.UI.OpenBrowser {
					if err := open(address); err != nil {
						_, writeErr := fmt.Fprintln(command.ErrOrStderr(), "Browser opening failed; open the printed URL manually.")
						return writeErr
					}
				}
				return nil
			}
			if d, err := discover(command.Context(), cfg); err == nil {
				return present(d)
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
			ui, d, err := startUI(command.Context(), cfg, "history", "")
			if err != nil {
				return err
			}
			defer func() { result = errors.Join(result, closeUI(cfg, ui, d)) }()
			if err = present(d); err != nil {
				return err
			}
			if _, err = fmt.Fprintln(command.ErrOrStderr(), "Serving local history. Press Ctrl+C to stop before running an application for this project."); err != nil {
				return err
			}
			<-ctx.Done()
			return nil
		}}
	command.Flags().StringVar(&configPath, "config", ".tracelet/config.yaml", "Project configuration path")
	command.Flags().BoolVar(&noOpen, "no-open", false, "Print the private local UI URL without opening a browser")
	return command
}

func openBrowser(address string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.CommandContext(ctx, "open", address)
	case "windows":
		command = exec.CommandContext(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", address)
	case "linux":
		command = exec.CommandContext(ctx, "xdg-open", address)
	default:
		return errors.New("browser opening is unavailable on this platform")
	}
	return command.Run()
}
