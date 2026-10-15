// Package cli is the cobra command tree and the CLI runtime contract:
// uniform global flags with GUPPY_* env binding, the exit-code table
// (design §6), and output policy (ADR 0009: stdlib plain text, diagnostics
// to stderr, data to stdout).
package cli

import "errors"

// Exit codes (design §6). These are user-facing contract: scripts and CI
// depend on them.
const (
	// ExitOK: success.
	ExitOK = 0
	// ExitFailure: the operation failed — lint/render errors and,
	// until a more specific code exists, unexpected internal errors.
	ExitFailure = 1
	// ExitInput: unresolved input — unknown command/flag, bad config file,
	// missing var in non-interactive mode, ambiguous template.
	ExitInput = 2
	// ExitNetwork: network/registry failure (offline violation, fetch or
	// checksum failure).
	ExitNetwork = 3
	// ExitInterrupted: SIGINT/SIGTERM aborted the run (shell convention:
	// 128 + signal number). Interruptible operations roll back before
	// this is returned — partial work is undone, never half-scaffolded
	// (styleguide U14, T14).
	ExitInterrupted = 130
)

// ExitError carries a specific exit code through cobra's error return.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// InputError marks err as an "unresolved input" failure (exit 2).
func InputError(err error) error { return &ExitError{Code: ExitInput, Err: err} }

// NetworkError marks err as a network/registry failure (exit 3).
func NetworkError(err error) error { return &ExitError{Code: ExitNetwork, Err: err} }

// Fail marks err as a plain operation failure (exit 1).
func Fail(err error) error { return &ExitError{Code: ExitFailure, Err: err} }

// exitCodeOf maps an error returned from command execution to an exit code.
// Unclassified errors default to ExitFailure.
func exitCodeOf(err error) int {
	if ee, ok := errors.AsType[*ExitError](err); ok {
		return ee.Code
	}
	return ExitFailure
}
