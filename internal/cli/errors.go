package cli

import (
	"errors"
	"fmt"
	"os"
)

type UsageError struct {
	Msg string
}

func (e *UsageError) Error() string {
	return e.Msg
}

type tuiNotImplementedError struct{}

func (tuiNotImplementedError) Error() string {
	return "TUI is not implemented yet"
}

func Main(args []string) int {
	root := newRootCommand()
	root.SetArgs(args)
	command, err := root.ExecuteC()
	if err == nil {
		return 0
	}

	var tuiErr tuiNotImplementedError
	if errors.As(err, &tuiErr) {
		return 1
	}

	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		fmt.Fprintf(os.Stderr, "sbxctl: %s\n", usageErr.Msg)
		if command == nil {
			command = root
		}
		fmt.Fprintf(os.Stderr, "Run '%s --help' for usage.\n", command.CommandPath())
		return 2
	}

	fmt.Fprintf(os.Stderr, "sbxctl: %s\n", err)
	return 1
}
