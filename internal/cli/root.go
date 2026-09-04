package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
	"github.com/spf13/cobra"
)

type rootFlags struct {
	Target    string
	URL       string
	Secret    string
	Output    string
	Timeout   time.Duration
	NoColor   bool
	targetSet bool
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
			flags.targetSet = cmd.Flags().Changed("target")
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
			fmt.Fprintln(cmd.ErrOrStderr(), "TUI is not implemented yet")
			return tuiNotImplementedError{}
		},
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &UsageError{Msg: err.Error()}
	})

	root.PersistentFlags().StringVar(&flags.Target, "target", "", "Use a saved target")
	root.PersistentFlags().StringVar(&flags.URL, "url", "", "Connect to an API URL")
	root.PersistentFlags().StringVar(&flags.Secret, "secret", "", "Authenticate with a secret")
	root.PersistentFlags().StringVar(&flags.Output, "output", "table", "Output format: table, json, or jsonl")
	root.PersistentFlags().DurationVar(&flags.Timeout, "timeout", 10*time.Second, "Request timeout")
	root.PersistentFlags().BoolVar(&flags.NoColor, "no-color", false, "Disable color output")

	root.AddCommand(
		newTargetCommand(flags),
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
