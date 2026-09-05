package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
	"github.com/legibet/sbxctl/internal/ui"
)

type rootFlags struct {
	Server    string
	URL       string
	Secret    string
	Output    string
	Timeout   time.Duration
	NoColor   bool
	serverSet bool
	urlSet    bool
	secretSet bool
}

func newRootCommand() *cobra.Command {
	flags := &rootFlags{}
	root := &cobra.Command{
		Use:           "sbxctl",
		Short:         "Terminal client for the sing-box API service",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return &UsageError{Msg: fmt.Sprintf("unknown command %q", args[0])}
			}
			return nil
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			flags.serverSet = cmd.Flags().Changed("server")
			flags.urlSet = cmd.Flags().Changed("url")
			flags.secretSet = cmd.Flags().Changed("secret")
			switch flags.Output {
			case "table", "json", "jsonl":
				return nil
			default:
				return &UsageError{Msg: fmt.Sprintf("invalid output format %q: expected table, json, or jsonl", flags.Output)}
			}
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, err := config.Load()
			if err != nil {
				return err
			}
			endpoint, err := resolveEndpoint(*flags, file)
			if err != nil {
				if errors.Is(err, errNoServer) {
					return ui.Run(sbx.Endpoint{}, "", file, flags.NoColor)
				}
				return err
			}
			return ui.Run(endpoint.Endpoint, endpoint.Name, file, flags.NoColor)
		},
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &UsageError{Msg: err.Error()}
	})

	root.PersistentFlags().StringVar(&flags.Server, "server", "", "Use a saved server for this invocation")
	root.PersistentFlags().StringVar(&flags.URL, "url", "", "Connect to an API URL")
	root.PersistentFlags().StringVar(&flags.Secret, "secret", "", "Authenticate with a secret")
	root.PersistentFlags().StringVar(&flags.Output, "output", "table", "Output format: table, json, or jsonl")
	root.PersistentFlags().DurationVar(&flags.Timeout, "timeout", 10*time.Second, "Request timeout")
	root.PersistentFlags().BoolVar(&flags.NoColor, "no-color", false, "Disable color output")

	root.AddCommand(
		newServerCommand(flags),
		newStatusCommand(flags),
		newGroupsCommand(flags),
		newOutboundsCommand(flags),
		newSelectCommand(flags),
		newTestCommand(flags),
		newModeCommand(flags),
		newConnectionsCommand(flags),
		newLogsCommand(flags),
	)
	return root
}

// connect resolves the endpoint, dials it and verifies the API version.
func connect(cmd *cobra.Command, flags *rootFlags) (*sbx.Client, resolved, error) {
	file, err := config.Load()
	if err != nil {
		return nil, resolved{}, err
	}
	endpoint, err := resolveEndpoint(*flags, file)
	if err != nil {
		return nil, resolved{}, err
	}
	client, err := sbx.Dial(endpoint.Endpoint)
	if err != nil {
		return nil, resolved{}, err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
	defer cancel()
	version, err := client.CheckVersion(ctx)
	if err != nil {
		client.Close()
		return nil, resolved{}, err
	}
	endpoint.version = version
	return client, endpoint, nil
}

func exactArgs(count int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return &UsageError{Msg: fmt.Sprintf("expected %d argument(s), got %d", count, len(args))}
		}
		return nil
	}
}

func maximumArgs(count int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > count {
			return &UsageError{Msg: fmt.Sprintf("expected at most %d argument(s), got %d", count, len(args))}
		}
		return nil
	}
}
