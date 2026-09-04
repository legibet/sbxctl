package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/legibet/sbxctl/internal/sbx"
	"github.com/spf13/cobra"
)

const connectionClosedLimit = 1000

func newConnectionsCommand(flags *rootFlags) *cobra.Command {
	var watch bool
	var state string
	var interval time.Duration
	command := &cobra.Command{
		Use:   "connections",
		Short: "List and manage connections",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if state != "active" && state != "closed" && state != "all" {
				return &UsageError{Msg: fmt.Sprintf("invalid connection state %q: expected active, closed, or all", state)}
			}
			if interval <= 0 {
				return &UsageError{Msg: "--interval must be positive"}
			}
			if watch {
				if err := validateStreamingOutput(flags.Output, "--watch"); err != nil {
					return err
				}
			}
			client, _, err := connect(cmd, flags)
			if err != nil {
				return err
			}
			defer client.Close()
			ctx, cancel := streamContext()
			defer cancel()
			tableState := sbx.NewConnectionTable(connectionClosedLimit, interval)
			frames := newFrameWriter(cmd.OutOrStdout(), flags.Output == "table", watch)
			err = client.WatchConnections(ctx, interval, func(batch sbx.ConnectionBatch) error {
				tableState.Apply(batch)
				connections := connectionsForState(tableState, state)
				if err := frames.Before(); err != nil {
					return err
				}
				if flags.Output == "table" {
					if err := writeConnectionsTable(cmd, connections, state, time.Now()); err != nil {
						return err
					}
				} else if err := writeJSON(cmd.OutOrStdout(), connections); err != nil {
					return err
				}
				if !watch {
					return sbx.ErrStop
				}
				return nil
			})
			return canceledStream(err, ctx)
		},
	}
	command.Flags().BoolVar(&watch, "watch", false, "Watch for changes")
	command.Flags().StringVar(&state, "state", "active", "Connection state: active, closed, or all")
	command.Flags().DurationVar(&interval, "interval", time.Second, "Traffic update interval")
	command.AddCommand(newConnectionShowCommand(flags), newConnectionCloseCommand(flags))
	return command
}

func connectionsForState(tableState *sbx.ConnectionTable, state string) []sbx.Connection {
	switch state {
	case "active":
		return tableState.Active()
	case "closed":
		return tableState.Closed()
	default:
		return tableState.All()
	}
}

func writeConnectionsTable(cmd *cobra.Command, connections []sbx.Connection, state string, now time.Time) error {
	upHeader, downHeader := "UP", "DOWN"
	if state != "active" {
		upHeader, downHeader = "UP_TOTAL", "DOWN_TOTAL"
	}
	t := newTable(
		cmd.OutOrStdout(),
		left("ID"), left("INBOUND"), left("SOURCE"), left("DESTINATION"), left("OUTBOUND"),
		right(upHeader), right(downHeader), right("AGE"),
	)
	for _, connection := range connections {
		destination := connection.Destination
		if connection.Domain != "" {
			destination = connection.Domain
		}
		up, down := connection.Uplink, connection.Downlink
		if state != "active" {
			up, down = connection.UplinkTotal, connection.DownlinkTotal
		}
		age := now.Sub(connection.CreatedAt)
		if !connection.ClosedAt.IsZero() {
			age = connection.ClosedAt.Sub(connection.CreatedAt)
		}
		t.Row(shortID(connection.ID), connection.Inbound, connection.Source, destination, connection.Outbound,
			formatRateOrTotal(up, state), formatRateOrTotal(down, state), formatShortDuration(age))
	}
	return t.Flush()
}

func formatRateOrTotal(value int64, state string) string {
	if state == "active" {
		return formatRate(value)
	}
	return formatBytes(value)
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func newConnectionShowCommand(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show connection details",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := connect(cmd, flags)
			if err != nil {
				return err
			}
			defer client.Close()
			connection, err := findConnection(client, args[0])
			if err != nil {
				return err
			}
			if flags.Output != "table" {
				return writeJSON(cmd.OutOrStdout(), connection)
			}
			return writeConnectionTable(cmd, connection)
		},
	}
}

func newConnectionCloseCommand(flags *rootFlags) *cobra.Command {
	var all bool
	var yes bool
	command := &cobra.Command{
		Use:   "close [id]",
		Short: "Close one or all connections",
		Args:  maximumArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all && len(args) == 1 {
				return &UsageError{Msg: "connection id cannot be combined with --all"}
			}
			if !all && len(args) == 0 {
				return &UsageError{Msg: "connection id or --all is required"}
			}
			if all && !yes {
				return &UsageError{Msg: "--all requires --yes"}
			}
			client, _, err := connect(cmd, flags)
			if err != nil {
				return err
			}
			defer client.Close()
			if all {
				ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
				defer cancel()
				if err := client.CloseAllConnections(ctx); err != nil {
					return err
				}
				if flags.Output != "table" {
					return writeJSON(cmd.OutOrStdout(), struct {
						All bool `json:"all"`
					}{All: true})
				}
				return nil
			}

			connection, err := findConnection(client, args[0])
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
			defer cancel()
			if err := client.CloseConnection(ctx, connection.ID); err != nil {
				return err
			}
			if flags.Output != "table" {
				return writeJSON(cmd.OutOrStdout(), struct {
					ID string `json:"id"`
				}{ID: connection.ID})
			}
			return nil
		},
	}
	command.Flags().BoolVar(&all, "all", false, "Close all connections")
	command.Flags().BoolVar(&yes, "yes", false, "Confirm closing all connections")
	return command
}

// findConnection takes one connections frame and resolves id, which may be a unique prefix.
func findConnection(client *sbx.Client, id string) (sbx.Connection, error) {
	ctx, cancel := streamContext()
	defer cancel()
	tableState := sbx.NewConnectionTable(connectionClosedLimit, time.Second)
	err := client.WatchConnections(ctx, time.Second, func(batch sbx.ConnectionBatch) error {
		tableState.Apply(batch)
		return sbx.ErrStop
	})
	if err != nil {
		return sbx.Connection{}, canceledStream(err, ctx)
	}
	matches := tableState.Find(id)
	switch len(matches) {
	case 0:
		return sbx.Connection{}, fmt.Errorf("connection %q not found", id)
	case 1:
		return matches[0], nil
	default:
		return sbx.Connection{}, fmt.Errorf("connection id %q is ambiguous", id)
	}
}

func writeConnectionTable(cmd *cobra.Command, connection sbx.Connection) error {
	t := newTable(cmd.OutOrStdout(), left(""), left(""))
	t.Row("id:", connection.ID)
	t.Row("inbound:", connection.Inbound)
	t.Row("inbound type:", connection.InboundType)
	t.Row("ip version:", strconv.Itoa(connection.IPVersion))
	t.Row("network:", connection.Network)
	t.Row("source:", connection.Source)
	t.Row("destination:", connection.Destination)
	t.Row("domain:", connection.Domain)
	t.Row("protocol:", connection.Protocol)
	t.Row("user:", connection.User)
	t.Row("from outbound:", connection.FromOutbound)
	t.Row("created at:", connection.CreatedAt.Format(time.RFC3339Nano))
	t.Row("closed at:", formatTime(connection.ClosedAt))
	t.Row("uplink:", formatRate(connection.Uplink))
	t.Row("downlink:", formatRate(connection.Downlink))
	t.Row("uplink total:", formatBytes(connection.UplinkTotal))
	t.Row("downlink total:", formatBytes(connection.DownlinkTotal))
	t.Row("rule:", connection.Rule)
	t.Row("outbound:", connection.Outbound)
	t.Row("outbound type:", connection.OutboundType)
	t.Row("chain:", strings.Join(connection.Chain, " > "))
	if connection.Process != nil {
		t.Row("process pid:", strconv.FormatUint(uint64(connection.Process.PID), 10))
		t.Row("process user id:", strconv.Itoa(connection.Process.UserID))
		t.Row("process user:", connection.Process.UserName)
		t.Row("process path:", connection.Process.Path)
		t.Row("process packages:", strings.Join(connection.Process.PackageNames, ", "))
	}
	return t.Flush()
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Format(time.RFC3339Nano)
}
