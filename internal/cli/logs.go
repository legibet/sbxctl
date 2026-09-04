package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/legibet/sbxctl/internal/sbx"
	"github.com/spf13/cobra"
)

func newLogsCommand(flags *rootFlags) *cobra.Command {
	var follow bool
	var levelName string
	var search string
	command := &cobra.Command{
		Use:   "logs",
		Short: "Show server logs",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if follow {
				if err := validateStreamingOutput(flags.Output, "--follow"); err != nil {
					return err
				}
			}
			var maxLevel sbx.LogLevel
			var err error
			if cmd.Flags().Changed("level") {
				maxLevel, err = sbx.ParseLogLevel(levelName)
				if err != nil {
					return &UsageError{Msg: err.Error()}
				}
			}
			client, _, err := connect(cmd, flags)
			if err != nil {
				return err
			}
			defer client.Close()
			if !cmd.Flags().Changed("level") {
				ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
				maxLevel, err = client.DefaultLogLevel(ctx)
				cancel()
				if err != nil {
					return err
				}
			}

			ctx, cancel := streamContext()
			defer cancel()
			color := colorsEnabled(flags)
			err = client.WatchLogs(ctx, func(batch sbx.LogBatch) error {
				entries := filterLogs(batch.Entries, maxLevel, search)
				if flags.Output == "table" {
					if err := writeLogTable(cmd, entries, color); err != nil {
						return err
					}
				} else if flags.Output == "json" {
					if err := writeJSON(cmd.OutOrStdout(), stripLogEntries(entries)); err != nil {
						return err
					}
				} else {
					for _, entry := range stripLogEntries(entries) {
						if err := writeJSON(cmd.OutOrStdout(), entry); err != nil {
							return err
						}
					}
				}
				if !follow {
					return sbx.ErrStop
				}
				return nil
			})
			return canceledStream(err, ctx)
		},
	}
	command.Flags().BoolVar(&follow, "follow", false, "Follow new log entries")
	command.Flags().StringVar(&levelName, "level", "", "Maximum log level: panic, fatal, error, warn, info, debug, or trace")
	command.Flags().StringVar(&search, "search", "", "Filter messages by text")
	command.AddCommand(newLogsClearCommand(flags))
	return command
}

func filterLogs(entries []sbx.LogEntry, maxLevel sbx.LogLevel, search string) []sbx.LogEntry {
	needle := strings.ToLower(search)
	filtered := make([]sbx.LogEntry, 0, len(entries))
	for _, entry := range entries {
		message := sbx.StripANSI(entry.Message)
		if entry.Level <= maxLevel && strings.Contains(strings.ToLower(message), needle) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func writeLogTable(cmd *cobra.Command, entries []sbx.LogEntry, color bool) error {
	for _, entry := range entries {
		message := entry.Message
		if !color {
			message = sbx.StripANSI(message)
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), message); err != nil {
			return err
		}
	}
	return nil
}

func stripLogEntries(entries []sbx.LogEntry) []sbx.LogEntry {
	stripped := make([]sbx.LogEntry, len(entries))
	for index, entry := range entries {
		entry.Message = sbx.StripANSI(entry.Message)
		stripped[index] = entry
	}
	return stripped
}

func newLogsClearCommand(flags *rootFlags) *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:   "clear",
		Short: "Clear buffered server logs",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes {
				return &UsageError{Msg: "logs clear requires --yes"}
			}
			client, _, err := connect(cmd, flags)
			if err != nil {
				return err
			}
			defer client.Close()
			ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
			defer cancel()
			if err := client.ClearLogs(ctx); err != nil {
				return err
			}
			if flags.Output == "table" {
				return nil
			}
			return writeJSON(cmd.OutOrStdout(), struct {
				Cleared bool `json:"cleared"`
			}{Cleared: true})
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "Confirm clearing logs")
	return command
}
