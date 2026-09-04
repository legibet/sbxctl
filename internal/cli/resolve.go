package cli

import (
	"fmt"
	"os"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
)

// errNoTarget is returned when nothing selects an endpoint; the root command
// uses it to open the TUI on the target picker instead of failing.
var errNoTarget = &UsageError{Msg: "no target configured: use 'sbxctl target add' or pass --url"}

type resolved struct {
	Name    string
	version sbx.Version
	sbx.Endpoint
}

func resolveEndpoint(flags rootFlags, file *config.File) (resolved, error) {
	if flags.urlSet && flags.targetSet {
		return resolved{}, &UsageError{Msg: "--url and --target cannot be used together"}
	}
	if flags.urlSet {
		return resolved{Endpoint: sbx.Endpoint{URL: flags.URL, Secret: flags.Secret}}, nil
	}

	targetName := flags.Target
	if !flags.targetSet {
		targetName = os.Getenv("SBXCTL_TARGET")
	}
	if flags.targetSet || targetName != "" {
		target, ok := file.Targets[targetName]
		if !ok {
			return resolved{}, &UsageError{Msg: fmt.Sprintf("unknown target %q", targetName)}
		}
		result := resolvedFromTarget(targetName, target)
		if flags.secretSet {
			result.Secret = flags.Secret
		}
		return result, nil
	}

	if envURL := os.Getenv("SBXCTL_URL"); envURL != "" {
		result := resolved{Endpoint: sbx.Endpoint{URL: envURL, Secret: os.Getenv("SBXCTL_SECRET")}}
		if flags.secretSet {
			result.Secret = flags.Secret
		}
		return result, nil
	}

	if file.Current != "" {
		if target, ok := file.Targets[file.Current]; ok {
			result := resolvedFromTarget(file.Current, target)
			if flags.secretSet {
				result.Secret = flags.Secret
			}
			return result, nil
		}
	}
	return resolved{}, errNoTarget
}

func resolvedFromTarget(name string, target config.Target) resolved {
	return resolved{
		Name: name,
		Endpoint: sbx.Endpoint{
			URL:        target.URL,
			Secret:     target.Secret,
			CAFile:     target.TLS.CAFile,
			ServerName: target.TLS.ServerName,
			Insecure:   target.TLS.Insecure,
		},
	}
}
