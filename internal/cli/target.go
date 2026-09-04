package cli

import (
	"fmt"
	"net/url"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
	"github.com/spf13/cobra"
)

func newTargetCommand(flags *rootFlags) *cobra.Command {
	command := &cobra.Command{Use: "target", Short: "Manage connection targets"}
	command.AddCommand(
		newTargetListCommand(flags),
		newTargetShowCommand(flags),
		newTargetAddCommand(flags),
		newTargetRemoveCommand(),
		newTargetUseCommand(),
	)
	return command
}

func newTargetListCommand(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved targets",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, err := config.Load()
			if err != nil {
				return err
			}
			items := make([]targetOutput, 0, len(file.Targets))
			for _, name := range file.Names() {
				items = append(items, targetOutputFor(name, file.Targets[name], file.Current == name))
			}
			if flags.Output != "table" {
				return writeJSON(cmd.OutOrStdout(), items)
			}
			writer := tableWriter(cmd.OutOrStdout())
			fmt.Fprintln(writer, "NAME\tURL\tCURRENT")
			for _, item := range items {
				current := ""
				if item.Current {
					current = "*"
				}
				fmt.Fprintf(writer, "%s\t%s\t%s\n", item.Name, item.URL, current)
			}
			return writer.Flush()
		},
	}
}

func newTargetShowCommand(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Show a saved target (default: current)",
		Args:  maximumArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := config.Load()
			if err != nil {
				return err
			}
			name := file.Current
			if len(args) == 1 {
				name = args[0]
			}
			if name == "" {
				return &UsageError{Msg: "no current target"}
			}
			target, ok := file.Targets[name]
			if !ok {
				return &UsageError{Msg: fmt.Sprintf("unknown target %q", name)}
			}
			item := targetOutputFor(name, target, file.Current == name)
			secret := "(none)"
			if target.Secret != "" {
				secret = "(set)"
			}
			if flags.Output != "table" {
				return writeJSON(cmd.OutOrStdout(), struct {
					targetOutput
					HasSecret bool `json:"has_secret"`
				}{targetOutput: item, HasSecret: target.Secret != ""})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "name:         %s\n", name)
			fmt.Fprintf(cmd.OutOrStdout(), "url:          %s\n", target.URL)
			fmt.Fprintf(cmd.OutOrStdout(), "current:      %t\n", item.Current)
			fmt.Fprintf(cmd.OutOrStdout(), "secret:       %s\n", secret)
			fmt.Fprintf(cmd.OutOrStdout(), "ca:           %s\n", target.TLS.CAFile)
			fmt.Fprintf(cmd.OutOrStdout(), "server name:  %s\n", target.TLS.ServerName)
			fmt.Fprintf(cmd.OutOrStdout(), "insecure:     %t\n", target.TLS.Insecure)
			return nil
		},
	}
}

func newTargetAddCommand(flags *rootFlags) *cobra.Command {
	var caFile string
	var serverName string
	var insecure bool
	command := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Save a new target",
		Args:  exactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			name, rawURL := args[0], args[1]
			if err := sbx.ParseURL(rawURL); err != nil {
				return &UsageError{Msg: err.Error()}
			}
			parsed, _ := url.Parse(rawURL)
			if insecure && parsed.Scheme == "http" {
				return &UsageError{Msg: "--insecure requires an https URL"}
			}
			file, err := config.Load()
			if err != nil {
				return err
			}
			if _, exists := file.Targets[name]; exists {
				return &UsageError{Msg: fmt.Sprintf("target %q already exists", name)}
			}
			file.Targets[name] = config.Target{
				URL:    rawURL,
				Secret: flags.Secret,
				TLS: config.TLS{
					CAFile:     caFile,
					ServerName: serverName,
					Insecure:   insecure,
				},
			}
			if len(file.Targets) == 1 {
				file.Current = name
			}
			return file.Save()
		},
	}
	command.Flags().StringVar(&caFile, "ca", "", "Trust certificates from a PEM file")
	command.Flags().StringVar(&serverName, "server-name", "", "Use a TLS server name")
	command.Flags().BoolVar(&insecure, "insecure", false, "Skip TLS certificate verification")
	return command
}

func newTargetRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a saved target",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			file, err := config.Load()
			if err != nil {
				return err
			}
			name := args[0]
			if _, exists := file.Targets[name]; !exists {
				return &UsageError{Msg: fmt.Sprintf("unknown target %q", name)}
			}
			delete(file.Targets, name)
			if file.Current == name {
				file.Current = ""
			}
			return file.Save()
		},
	}
}

func newTargetUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Set the current target",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			file, err := config.Load()
			if err != nil {
				return err
			}
			name := args[0]
			if _, exists := file.Targets[name]; !exists {
				return &UsageError{Msg: fmt.Sprintf("unknown target %q", name)}
			}
			file.Current = name
			return file.Save()
		},
	}
}

type targetTLSOutput struct {
	CAFile     string `json:"ca_file,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	Insecure   bool   `json:"insecure,omitempty"`
}

type targetOutput struct {
	Name    string          `json:"name"`
	URL     string          `json:"url"`
	Current bool            `json:"current"`
	TLS     targetTLSOutput `json:"tls"`
}

func targetOutputFor(name string, target config.Target, current bool) targetOutput {
	return targetOutput{
		Name:    name,
		URL:     target.URL,
		Current: current,
		TLS: targetTLSOutput{
			CAFile:     target.TLS.CAFile,
			ServerName: target.TLS.ServerName,
			Insecure:   target.TLS.Insecure,
		},
	}
}
