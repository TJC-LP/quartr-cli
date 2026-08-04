package cli

import "fmt"

// usageError marks a failure caused by the command line the user typed
// rather than by the API or the network. Run maps it to exit code 2 so
// scripts can tell "you asked for something impossible" apart from "the
// request failed".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}
