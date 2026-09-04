package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/legibet/sbxctl/internal/sbx"
	"github.com/spf13/cobra"
)

func newSelectCommand(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "select <group> <outbound>",
		Short: "Select an outbound in a group",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := connect(cmd, flags)
			if err != nil {
				return err
			}
			defer client.Close()
			ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
			defer cancel()
			if err := client.SelectOutbound(ctx, args[0], args[1]); err != nil {
				return err
			}
			if flags.Output == "table" {
				return nil
			}
			return writeJSON(cmd.OutOrStdout(), struct {
				Group    string `json:"group"`
				Outbound string `json:"outbound"`
			}{Group: args[0], Outbound: args[1]})
		},
	}
}

func newTestCommand(flags *rootFlags) *cobra.Command {
	wait := 15 * time.Second
	command := &cobra.Command{
		Use:   "test <outbound-or-group>",
		Short: "Trigger a URL test",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := connect(cmd, flags)
			if err != nil {
				return err
			}
			defer client.Close()
			if !cmd.Flags().Changed("wait") {
				ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
				defer cancel()
				if err := client.URLTest(ctx, args[0]); err != nil {
					return err
				}
				if flags.Output == "table" {
					_, err := fmt.Fprintln(cmd.OutOrStdout(), "triggered url test for "+args[0])
					return err
				}
				return writeJSON(cmd.OutOrStdout(), struct {
					Tag       string `json:"tag"`
					Triggered bool   `json:"triggered"`
				}{Tag: args[0], Triggered: true})
			}
			if wait <= 0 {
				return &UsageError{Msg: "--wait must be positive"}
			}
			return runTestAndWait(cmd, flags, client, args[0], wait)
		},
	}
	command.Flags().DurationVar(&wait, "wait", wait, "Wait for URL test results")
	return command
}

type testSnapshot struct {
	groups    []sbx.Group
	outbounds []sbx.Outbound
	source    string
	err       error
}

func runTestAndWait(cmd *cobra.Command, flags *rootFlags, client *sbx.Client, tag string, wait time.Duration) error {
	ctx, cancel := streamContext()
	defer cancel()
	snapshots := make(chan testSnapshot, 16)
	go func() {
		err := client.WatchGroups(ctx, func(groups []sbx.Group) error {
			snapshot := testSnapshot{groups: groups, outbounds: flattenGroups(groups), source: "groups"}
			select {
			case snapshots <- snapshot:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		select {
		case snapshots <- testSnapshot{source: "groups", err: err}:
		case <-ctx.Done():
		}
	}()
	go func() {
		err := client.WatchOutbounds(ctx, func(outbounds []sbx.Outbound) error {
			select {
			case snapshots <- testSnapshot{outbounds: outbounds, source: "outbounds"}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		select {
		case snapshots <- testSnapshot{source: "outbounds", err: err}:
		case <-ctx.Done():
		}
	}()

	var groups []sbx.Group
	var groupItems, outbounds []sbx.Outbound
	haveGroups, haveOutbounds := false, false
	for !haveGroups || !haveOutbounds {
		var snapshot testSnapshot
		select {
		case snapshot = <-snapshots:
		case <-ctx.Done():
			return nil
		}
		if snapshot.err != nil {
			return canceledStream(snapshot.err, ctx)
		}
		switch snapshot.source {
		case "groups":
			groups = snapshot.groups
			groupItems = snapshot.outbounds
			haveGroups = true
		case "outbounds":
			outbounds = snapshot.outbounds
			haveOutbounds = true
		}
	}

	before := outboundMap(groupItems, outbounds)
	tags := []string{tag}
	if group, ok := findGroup(groups, tag); ok {
		tags = make([]string, len(group.Items))
		for index, item := range group.Items {
			tags[index] = item.Tag
		}
	} else if _, ok := before[tag]; !ok {
		return fmt.Errorf("outbound %q not found", tag)
	}

	startedAt := time.Now()
	tracker := sbx.NewTestTracker(tags, before, startedAt, wait)
	callCtx, callCancel := context.WithTimeout(cmd.Context(), flags.Timeout)
	err := client.URLTest(callCtx, tag)
	callCancel()
	if err != nil {
		return err
	}

	timer := time.NewTimer(time.Until(startedAt.Add(wait)))
	defer timer.Stop()
	for !tracker.Done() {
		select {
		case snapshot := <-snapshots:
			if snapshot.err != nil {
				return canceledStream(snapshot.err, ctx)
			}
			tracker.Observe(snapshot.outbounds, time.Now())
		case now := <-timer.C:
			tracker.Observe(nil, now)
		case <-ctx.Done():
			return nil
		}
	}
	results := tracker.Results()
	if flags.Output != "table" {
		return writeJSON(cmd.OutOrStdout(), results)
	}
	t := newTable(cmd.OutOrStdout(), left("TAG"), left("RESULT"), right("DELAY"))
	for _, result := range results {
		state, _ := result.State.MarshalText()
		delay := "-"
		if result.State == sbx.TestOK {
			delay = fmt.Sprintf("%dms", result.Delay)
		}
		t.Row(result.Tag, string(state), delay)
	}
	return t.Flush()
}

func flattenGroups(groups []sbx.Group) []sbx.Outbound {
	var outbounds []sbx.Outbound
	for _, group := range groups {
		outbounds = append(outbounds, group.Items...)
	}
	return outbounds
}

func outboundMap(lists ...[]sbx.Outbound) map[string]sbx.Outbound {
	items := make(map[string]sbx.Outbound)
	for _, list := range lists {
		for _, item := range list {
			current, exists := items[item.Tag]
			if !exists || item.TestedAt.After(current.TestedAt) {
				items[item.Tag] = item
			}
		}
	}
	return items
}

func newModeCommand(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "mode [name]",
		Short: "Show or set Clash mode",
		Args:  maximumArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := connect(cmd, flags)
			if err != nil {
				return err
			}
			defer client.Close()
			ctx, cancel := context.WithTimeout(cmd.Context(), flags.Timeout)
			defer cancel()
			if len(args) == 1 {
				if err := client.SetClashMode(ctx, args[0]); err != nil {
					return err
				}
			}
			mode, err := client.ClashMode(ctx)
			if err != nil {
				return err
			}
			if flags.Output != "table" {
				return writeJSON(cmd.OutOrStdout(), mode)
			}
			t := newTable(cmd.OutOrStdout(), left(""), left(""))
			for _, name := range mode.Modes {
				selected := ""
				if name == mode.Current {
					selected = "*"
				}
				t.Row(selected, name)
			}
			return t.Flush()
		},
	}
}
