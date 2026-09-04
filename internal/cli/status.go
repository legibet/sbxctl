package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/legibet/sbxctl/internal/config"
	"github.com/legibet/sbxctl/internal/sbx"
	"github.com/spf13/cobra"
)

type statusOutput struct {
	Target     string                  `json:"target"`
	URL        string                  `json:"url"`
	Version    string                  `json:"version"`
	APIVersion int                     `json:"api_version"`
	StartedAt  time.Time               `json:"started_at"`
	Mode       *sbx.ClashMode          `json:"mode"`
	Status     sbx.Status              `json:"status"`
	Warnings   []sbx.DeprecatedWarning `json:"warnings"`
}

func newStatusCommand(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show server version, uptime, traffic and resources",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, err := config.Load()
			if err != nil {
				return err
			}
			endpoint, err := resolveEndpoint(*flags, file)
			if err != nil {
				return err
			}
			client, err := sbx.Dial(endpoint.Endpoint)
			if err != nil {
				return err
			}
			defer client.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
			defer cancel()
			version, err := client.CheckVersion(ctx)
			if err != nil {
				return err
			}
			startedAt, err := client.StartedAt(ctx)
			if err != nil {
				return err
			}
			warnings, err := client.DeprecatedWarnings(ctx)
			if err != nil {
				return err
			}
			mode, err := client.ClashMode(ctx)
			var outputMode *sbx.ClashMode
			if err == nil {
				outputMode = &mode
			} else if sbx.KindOf(err) != sbx.KindUnsupported {
				return err
			}
			var currentStatus sbx.Status
			if err := client.WatchStatus(ctx, 0, func(status sbx.Status) error {
				currentStatus = status
				return sbx.ErrStop
			}); err != nil {
				return err
			}

			output := statusOutput{
				Target:     endpoint.Name,
				URL:        endpoint.URL,
				Version:    version.Version,
				APIVersion: version.APIVersion,
				StartedAt:  startedAt,
				Mode:       outputMode,
				Status:     currentStatus,
				Warnings:   warnings,
			}
			if flags.Output != "table" {
				return writeJSON(cmd.OutOrStdout(), output)
			}
			writeStatusTable(cmd, output)
			return nil
		},
	}
}

func writeStatusTable(cmd *cobra.Command, output statusOutput) {
	target := output.URL
	if output.Target != "" {
		target = fmt.Sprintf("%s (%s)", output.Target, output.URL)
	}
	printStatusLine(cmd, "target:", target)
	printStatusLine(cmd, "version:", fmt.Sprintf("%s (api %d)", output.Version, output.APIVersion))
	printStatusLine(cmd, "uptime:", formatDuration(time.Since(output.StartedAt)))
	if output.Mode != nil {
		printStatusLine(cmd, "mode:", fmt.Sprintf("%s (%s)", output.Mode.Current, strings.Join(output.Mode.Modes, ", ")))
	}
	if output.Status.TrafficAvailable {
		printStatusLine(cmd, "traffic:", fmt.Sprintf("↑ %s  ↓ %s", formatRate(output.Status.Uplink), formatRate(output.Status.Downlink)))
		printStatusLine(cmd, "total:", fmt.Sprintf("↑ %s  ↓ %s", formatBytes(output.Status.UplinkTotal), formatBytes(output.Status.DownlinkTotal)))
		printStatusLine(cmd, "connections:", fmt.Sprintf("%d in, %d out", output.Status.ConnectionsIn, output.Status.ConnectionsOut))
	}
	printStatusLine(cmd, "memory:", formatBytes(int64(output.Status.Memory)))
	printStatusLine(cmd, "goroutines:", fmt.Sprintf("%d", output.Status.Goroutines))
	if len(output.Warnings) > 0 {
		printStatusLine(cmd, "warnings:", fmt.Sprintf("%d", len(output.Warnings)))
		for _, warning := range output.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", warning.Message)
		}
	}
}

func printStatusLine(cmd *cobra.Command, label, value string) {
	fmt.Fprintf(cmd.OutOrStdout(), "%-14s%s\n", label, value)
}
