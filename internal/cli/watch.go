package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func validateStreamingOutput(output, flag string) error {
	if output == "json" {
		return &UsageError{Msg: fmt.Sprintf("--output json cannot be combined with %s; use jsonl", flag)}
	}
	return nil
}

type frameWriter struct {
	w        io.Writer
	table    bool
	watch    bool
	terminal bool
	written  bool
}

func newFrameWriter(w io.Writer, tableOutput, watch bool) *frameWriter {
	return &frameWriter{w: w, table: tableOutput, watch: watch, terminal: stdoutIsTerminal()}
}

func (f *frameWriter) Before() error {
	if f.table && f.watch {
		if f.terminal {
			if _, err := io.WriteString(f.w, "\x1b[H\x1b[2J"); err != nil {
				return err
			}
		} else if f.written {
			if _, err := io.WriteString(f.w, "\n"); err != nil {
				return err
			}
		}
	}
	f.written = true
	return nil
}

func streamContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func canceledStream(err error, ctx context.Context) error {
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		err = nil
	}
	return err
}
