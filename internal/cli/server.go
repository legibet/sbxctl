package cli

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
)

func newServerCommand(flags *rootFlags) *cobra.Command {
	command := &cobra.Command{Use: "server", Short: "Manage saved sing-box API servers"}
	command.AddCommand(
		newServerListCommand(flags),
		newServerShowCommand(flags),
		newServerAddCommand(flags),
		newServerRemoveCommand(),
		newServerUseCommand(),
	)
	return command
}

func newServerListCommand(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved servers",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, err := config.Load()
			if err != nil {
				return err
			}
			items := make([]serverOutput, 0, len(file.Servers))
			for _, name := range file.Names() {
				items = append(items, serverOutputFor(name, file.Servers[name], file.Current == name))
			}
			if flags.Output != "table" {
				return writeJSON(cmd.OutOrStdout(), items)
			}
			t := newTable(cmd.OutOrStdout(), left("NAME"), left("URL"), left("CURRENT"))
			for _, item := range items {
				current := ""
				if item.Current {
					current = "*"
				}
				t.Row(item.Name, item.URL, current)
			}
			return t.Flush()
		},
	}
}

func newServerShowCommand(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Show a saved server (default: current)",
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
				return &UsageError{Msg: "no current server: use 'sbxctl server use <name>'"}
			}
			server, ok := file.Servers[name]
			if !ok {
				return &UsageError{Msg: fmt.Sprintf("unknown server %q", name)}
			}
			item := serverOutputFor(name, server, file.Current == name)
			secret := "(none)"
			if server.Secret != "" {
				secret = "(set)"
			}
			if flags.Output != "table" {
				return writeJSON(cmd.OutOrStdout(), struct {
					serverOutput
					HasSecret bool `json:"has_secret"`
				}{serverOutput: item, HasSecret: server.Secret != ""})
			}
			t := newTable(cmd.OutOrStdout(), left(""), left(""))
			t.Row("name:", name)
			t.Row("url:", server.URL)
			t.Row("current:", fmt.Sprint(item.Current))
			t.Row("secret:", secret)
			t.Row("ca:", server.TLS.CAFile)
			t.Row("server name:", server.TLS.ServerName)
			t.Row("insecure:", fmt.Sprint(server.TLS.Insecure))
			return t.Flush()
		},
	}
}

func newServerAddCommand(flags *rootFlags) *cobra.Command {
	var caFile string
	var serverName string
	var insecure bool
	command := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Save an existing sing-box API server",
		Args:  exactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			name, rawURL := strings.TrimSpace(args[0]), strings.TrimSpace(args[1])
			if name == "" || strings.ContainsFunc(name, unicode.IsControl) {
				return &UsageError{Msg: "enter a server name without control characters"}
			}
			if err := sbx.ParseURL(rawURL); err != nil {
				return &UsageError{Msg: err.Error()}
			}
			parsed, _ := url.Parse(rawURL)
			if (insecure || caFile != "" || serverName != "") && parsed.Scheme == "http" {
				return &UsageError{Msg: "TLS settings require an https URL"}
			}
			file, err := config.Load()
			if err != nil {
				return err
			}
			if _, exists := file.Servers[name]; exists {
				return &UsageError{Msg: fmt.Sprintf("server %q already exists", name)}
			}
			file.Servers[name] = config.Server{
				URL:    rawURL,
				Secret: flags.Secret,
				TLS: config.TLS{
					CAFile:     caFile,
					ServerName: serverName,
					Insecure:   insecure,
				},
			}
			if len(file.Servers) == 1 {
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

func newServerRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a local server record",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			file, err := config.Load()
			if err != nil {
				return err
			}
			name := args[0]
			if _, exists := file.Servers[name]; !exists {
				return &UsageError{Msg: fmt.Sprintf("unknown server %q", name)}
			}
			delete(file.Servers, name)
			if file.Current == name {
				file.Current = ""
			}
			return file.Save()
		},
	}
}

func newServerUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Choose the server for subsequent invocations",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			file, err := config.Load()
			if err != nil {
				return err
			}
			name := args[0]
			if _, exists := file.Servers[name]; !exists {
				return &UsageError{Msg: fmt.Sprintf("unknown server %q", name)}
			}
			file.Current = name
			return file.Save()
		},
	}
}

type serverTLSOutput struct {
	CAFile     string `json:"ca_file,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	Insecure   bool   `json:"insecure,omitempty"`
}

type serverOutput struct {
	Name    string          `json:"name"`
	URL     string          `json:"url"`
	Current bool            `json:"current"`
	TLS     serverTLSOutput `json:"tls"`
}

func serverOutputFor(name string, server config.Server, current bool) serverOutput {
	return serverOutput{
		Name:    name,
		URL:     server.URL,
		Current: current,
		TLS: serverTLSOutput{
			CAFile:     server.TLS.CAFile,
			ServerName: server.TLS.ServerName,
			Insecure:   server.TLS.Insecure,
		},
	}
}
