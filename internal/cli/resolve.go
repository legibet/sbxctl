package cli

import (
	"fmt"
	"os"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
)

// The root command opens the server manager when no endpoint is selected.
var errNoServer = &UsageError{Msg: "no server selected: use 'sbxctl server use <name>', add one with 'sbxctl server add', or pass --url"}

type resolved struct {
	Name    string
	version sbx.Version
	sbx.Endpoint
}

func resolveEndpoint(flags rootFlags, file *config.File) (resolved, error) {
	if flags.urlSet && flags.serverSet {
		return resolved{}, &UsageError{Msg: "--url and --server cannot be used together"}
	}
	if flags.urlSet {
		return resolved{URL: flags.URL, Secret: flags.Secret}, nil
	}

	serverName := flags.Server
	if !flags.serverSet {
		serverName = os.Getenv("SBXCTL_SERVER")
	}
	if flags.serverSet || serverName != "" {
		server, ok := file.Servers[serverName]
		if !ok {
			return resolved{}, &UsageError{Msg: fmt.Sprintf("unknown server %q", serverName)}
		}
		result := resolvedFromServer(serverName, server)
		if flags.secretSet {
			result.Secret = flags.Secret
		}
		return result, nil
	}

	if envURL := os.Getenv("SBXCTL_URL"); envURL != "" {
		result := resolved{URL: envURL, Secret: os.Getenv("SBXCTL_SECRET")}
		if flags.secretSet {
			result.Secret = flags.Secret
		}
		return result, nil
	}

	if file.Current != "" {
		if server, ok := file.Servers[file.Current]; ok {
			result := resolvedFromServer(file.Current, server)
			if flags.secretSet {
				result.Secret = flags.Secret
			}
			return result, nil
		}
	}
	return resolved{}, errNoServer
}

func resolvedFromServer(name string, server config.Server) resolved {
	return resolved{
		Name:       name,
		URL:        server.URL,
		Secret:     server.Secret,
		CAFile:     server.TLS.CAFile,
		ServerName: server.TLS.ServerName,
		Insecure:   server.TLS.Insecure,
	}
}
