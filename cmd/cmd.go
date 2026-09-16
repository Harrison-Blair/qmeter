// Package cmd wires the qmeter command tree. The module root's main.go is the
// installable entrypoint and only calls Execute.
package cmd

import "io"

// Execute builds the root command and runs it against os.Args, writing to the
// process's own stdout and stderr. It is the entrypoint main.go calls.
func Execute() error {
	return NewRootCmd().Execute()
}

// ExecuteWithArgs is Execute with the arguments and the output writer injected,
// so a test can drive the whole command tree without touching os.Args or the
// process's streams. Both stdout and stderr of the command tree go to out.
func ExecuteWithArgs(args []string, out io.Writer) error {
	root := NewRootCmd()
	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(out)
	return root.Execute()
}
