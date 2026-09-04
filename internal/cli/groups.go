package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/legibet/sbxctl/internal/sbx"
	"github.com/spf13/cobra"
)

func newGroupsCommand(flags *rootFlags) *cobra.Command {
	var watch bool
	command := &cobra.Command{
		Use:   "groups",
		Short: "List outbound groups",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGroups(cmd, flags, watch, "")
		},
	}
	command.Flags().BoolVar(&watch, "watch", false, "Watch for changes")
	command.AddCommand(newGroupShowCommand(flags))
	return command
}

func newGroupShowCommand(flags *rootFlags) *cobra.Command {
	var watch bool
	command := &cobra.Command{
		Use:   "show <group>",
		Short: "Show an outbound group",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGroups(cmd, flags, watch, args[0])
		},
	}
	command.Flags().BoolVar(&watch, "watch", false, "Watch for changes")
	return command
}

func runGroups(cmd *cobra.Command, flags *rootFlags, watch bool, groupTag string) error {
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
	frames := newFrameWriter(cmd.OutOrStdout(), flags.Output == "table", watch)
	err = client.WatchGroups(ctx, func(groups []sbx.Group) error {
		if err := frames.Before(); err != nil {
			return err
		}
		if groupTag == "" {
			if flags.Output == "table" {
				if err := writeGroupsTable(cmd, groups); err != nil {
					return err
				}
			} else if err := writeJSON(cmd.OutOrStdout(), groups); err != nil {
				return err
			}
		} else {
			group, ok := findGroup(groups, groupTag)
			if !ok {
				return fmt.Errorf("group %q not found", groupTag)
			}
			if flags.Output == "table" {
				if err := writeGroupTable(cmd, group, time.Now()); err != nil {
					return err
				}
			} else if err := writeJSON(cmd.OutOrStdout(), group); err != nil {
				return err
			}
		}
		if !watch {
			return sbx.ErrStop
		}
		return nil
	})
	return canceledStream(err, ctx)
}

func findGroup(groups []sbx.Group, tag string) (sbx.Group, bool) {
	for _, group := range groups {
		if group.Tag == tag {
			return group, true
		}
	}
	return sbx.Group{}, false
}

func writeGroupsTable(cmd *cobra.Command, groups []sbx.Group) error {
	t := newTable(cmd.OutOrStdout(), left("GROUP"), left("TYPE"), left("SELECTED"), right("ITEMS"))
	for _, group := range groups {
		t.Row(group.Tag, group.Type, group.Selected, strconv.Itoa(len(group.Items)))
	}
	return t.Flush()
}

func writeGroupTable(cmd *cobra.Command, group sbx.Group, now time.Time) error {
	summary := newTable(cmd.OutOrStdout(), left(""), left(""), left(""))
	summary.Row(group.Tag, group.Type, "selected: "+group.Selected)
	if err := summary.Flush(); err != nil {
		return err
	}
	if err := summary.Blank(); err != nil {
		return err
	}
	t := newTable(cmd.OutOrStdout(), left(""), left("TAG"), left("TYPE"), right("DELAY"), right("TESTED"))
	for _, item := range group.Items {
		selected := ""
		if item.Tag == group.Selected {
			selected = "*"
		}
		t.Row(selected, item.Tag, item.Type, formatDelay(item), formatRelativeTime(item.TestedAt, now))
	}
	return t.Flush()
}

func formatDelay(item sbx.Outbound) string {
	if item.TestedAt.IsZero() {
		return "-"
	}
	return fmt.Sprintf("%dms", item.Delay)
}

func newOutboundsCommand(flags *rootFlags) *cobra.Command {
	var watch bool
	command := &cobra.Command{
		Use:   "outbounds",
		Short: "List outbounds and endpoints",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			frames := newFrameWriter(cmd.OutOrStdout(), flags.Output == "table", watch)
			err = client.WatchOutbounds(ctx, func(outbounds []sbx.Outbound) error {
				if err := frames.Before(); err != nil {
					return err
				}
				if flags.Output == "table" {
					if err := writeOutboundsTable(cmd, outbounds, time.Now()); err != nil {
						return err
					}
				} else if err := writeJSON(cmd.OutOrStdout(), outbounds); err != nil {
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
	return command
}

func writeOutboundsTable(cmd *cobra.Command, outbounds []sbx.Outbound, now time.Time) error {
	t := newTable(cmd.OutOrStdout(), left("TAG"), left("TYPE"), right("DELAY"), right("TESTED"))
	for _, outbound := range outbounds {
		t.Row(outbound.Tag, outbound.Type, formatDelay(outbound), formatRelativeTime(outbound.TestedAt, now))
	}
	return t.Flush()
}
