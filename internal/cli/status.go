package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/legibet/sbxctl/internal/sbx"
)

type statusOutput struct {
	Server     string                  `json:"server"`
	URL        string                  `json:"url"`
	Version    string                  `json:"version"`
	APIVersion int                     `json:"api_version"`
	StartedAt  time.Time               `json:"started_at,omitzero"`
	Mode       *sbx.ClashMode          `json:"mode"`
	Status     sbx.Status              `json:"status"`
	Warnings   []sbx.DeprecatedWarning `json:"warnings"`
}

func newStatusCommand(flags *rootFlags) *cobra.Command {
	var watch bool
	var interval time.Duration
	command := &cobra.Command{
		Use:   "status",
		Short: "Show server version, uptime, traffic and resources",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if interval <= 0 {
				return &UsageError{Msg: "--interval must be positive"}
			}
			if watch {
				if err := validateStreamingOutput(flags.Output, "--watch"); err != nil {
					return err
				}
			}
			client, endpoint, err := connect(cmd, flags)
			if err != nil {
				return err
			}
			defer client.Close()

			ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
			defer cancel()
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
			base := statusOutput{
				Server:     endpoint.Name,
				URL:        endpoint.URL,
				Version:    endpoint.version.Version,
				APIVersion: endpoint.version.APIVersion,
				StartedAt:  startedAt,
				Mode:       outputMode,
				Warnings:   warnings,
			}

			streamCtx, streamCancel := streamContext()
			defer streamCancel()
			frames := newFrameWriter(cmd.OutOrStdout(), flags.Output == "table", watch)
			err = client.WatchStatus(streamCtx, interval, func(status sbx.Status) error {
				output := base
				output.Status = status
				if err := frames.Before(); err != nil {
					return err
				}
				if flags.Output == "table" {
					if err := writeStatusTable(cmd, output); err != nil {
						return err
					}
				} else if err := writeJSON(cmd.OutOrStdout(), output); err != nil {
					return err
				}
				if !watch {
					return sbx.ErrStop
				}
				return nil
			})
			return canceledStream(err, streamCtx)
		},
	}
	command.Flags().BoolVar(&watch, "watch", false, "Watch for changes")
	command.Flags().DurationVar(&interval, "interval", time.Second, "Status update interval")
	return command
}

func writeStatusTable(cmd *cobra.Command, output statusOutput) error {
	t := newTable(cmd.OutOrStdout(), left(""), left(""))
	server := output.URL
	if output.Server != "" {
		server = fmt.Sprintf("%s (%s)", output.Server, output.URL)
	}
	t.Row("server:", server)
	t.Row("version:", fmt.Sprintf("%s (api %d)", output.Version, output.APIVersion))
	uptime := "-"
	if !output.StartedAt.IsZero() {
		uptime = sbx.FormatDuration(time.Since(output.StartedAt))
	}
	t.Row("uptime:", uptime)
	if output.Mode != nil {
		t.Row("mode:", fmt.Sprintf("%s (%s)", output.Mode.Current, strings.Join(output.Mode.Modes, ", ")))
	}
	if output.Status.TrafficAvailable {
		t.Row("traffic:", fmt.Sprintf("↑ %s  ↓ %s", sbx.FormatRate(output.Status.Uplink), sbx.FormatRate(output.Status.Downlink)))
		t.Row("total:", fmt.Sprintf("↑ %s  ↓ %s", sbx.FormatBytes(output.Status.UplinkTotal), sbx.FormatBytes(output.Status.DownlinkTotal)))
		t.Row("connections:", fmt.Sprintf("%d in, %d out", output.Status.ConnectionsIn, output.Status.ConnectionsOut))
	}
	t.Row("memory:", sbx.FormatBytes(int64(output.Status.Memory)))
	t.Row("goroutines:", strconv.Itoa(output.Status.Goroutines))
	if len(output.Warnings) > 0 {
		t.Row("warnings:", strconv.Itoa(len(output.Warnings)))
		for _, warning := range output.Warnings {
			t.Row("", "- "+warning.Message)
		}
	}
	return t.Flush()
}
