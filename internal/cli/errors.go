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

func Main(args []string) int {
	root := newRootCommand()
	root.SetArgs(args)
	command, err := root.ExecuteC()
	if err == nil {
		return 0
	}

	if usageErr, ok := errors.AsType[*UsageError](err); ok {
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
