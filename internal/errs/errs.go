// Package errs defines pixopher's structured errors. Error codes and their
// exit codes are public API (see docs/cli.md). This package has no internal
// dependencies so every other package can use it.
package errs

import (
	"errors"
	"fmt"
)

// Code is a stable, machine-readable error code.
type Code string

// Error codes from docs/cli.md. Adding a code is a minor change; renaming or
// removing one is breaking.
const (
	Usage              Code = "usage"
	NoFile             Code = "no_file"
	FileExists         Code = "file_exists"
	ParseError         Code = "parse_error"
	UnsupportedVersion Code = "unsupported_version"
	OutOfBounds        Code = "out_of_bounds"
	InvalidColor       Code = "invalid_color"
	UnknownColor       Code = "unknown_color"
	UnknownLayer       Code = "unknown_layer"
	UnknownFrame       Code = "unknown_frame"
	UnknownRegion      Code = "unknown_region"
	UnknownAnchor      Code = "unknown_anchor"
	UnknownPart        Code = "unknown_part"
	LayerLocked        Code = "layer_locked"
	RevConflict        Code = "rev_conflict"
	Locked             Code = "locked"
	UndoConflict       Code = "undo_conflict"
	NoHistory          Code = "no_history"
	TooLarge           Code = "too_large"
	PaletteFull        Code = "palette_full"
	GridMismatch       Code = "grid_mismatch"
	ScriptError        Code = "script_error"
	IO                 Code = "io"
	UnsupportedFormat  Code = "unsupported_format"
	ServerUnavailable  Code = "server_unavailable"
	AlreadyExists      Code = "already_exists" // a key or name is taken (D53)
	InUse              Code = "in_use"         // removing something still used or referenced (D53)
)

// All returns every error code, in the order docs/cli.md lists them.
func All() []Code {
	return []Code{
		Usage, NoFile, FileExists, ParseError, UnsupportedVersion,
		OutOfBounds, InvalidColor, UnknownColor, UnknownLayer,
		UnknownFrame, UnknownRegion, UnknownAnchor, UnknownPart,
		LayerLocked, RevConflict, Locked, UndoConflict, NoHistory, TooLarge,
		PaletteFull, GridMismatch, ScriptError, IO, UnsupportedFormat,
		ServerUnavailable, AlreadyExists, InUse,
	}
}

// Process exit codes from docs/cli.md.
const (
	ExitOK       = 0
	ExitUsage    = 1
	ExitDomain   = 2
	ExitConflict = 3
	ExitIO       = 4
	ExitLint     = 5
)

// ExitCode maps an error code to its process exit code.
func ExitCode(c Code) int {
	switch c {
	case Usage:
		return ExitUsage
	case RevConflict, Locked, UndoConflict:
		return ExitConflict
	case IO, NoFile, ServerUnavailable:
		return ExitIO
	default:
		return ExitDomain
	}
}

// Error is a structured error with a stable code. Message says what went
// wrong, Hint (optional) says how to fix it, and Arg (optional) names the
// argument at fault. Details (optional) carries code-specific data, such as
// the overlapping pixels of an undo_conflict. A script_error also has Line,
// the script line at fault, and Cause, the code of the error it wraps
// (AtLine). Its JSON form is the "error" object of the --json error
// envelope (docs/cli.md#output-done).
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	Arg     string `json:"arg,omitempty"`
	Line    int    `json:"line,omitempty"`
	Cause   Code   `json:"cause,omitempty"`
	Details any    `json:"details,omitempty"`
}

func (e *Error) Error() string { return e.Message }

// New returns an Error with a formatted message.
func New(code Code, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...)}
}

// WithHint sets the hint and returns e.
func (e *Error) WithHint(format string, a ...any) *Error {
	e.Hint = fmt.Sprintf(format, a...)
	return e
}

// WithDetails sets the details and returns e.
func (e *Error) WithDetails(d any) *Error {
	e.Details = d
	return e
}

// WithArg sets the argument name and returns e.
func (e *Error) WithArg(arg string) *Error {
	e.Arg = arg
	return e
}

// AtLine wraps err, the failure of line n of a batch script, in a
// script_error: the message gains a `line <n>: <code>: ` prefix, Line is n
// and Cause is err's code, and the hint, arg and details carry over. An err
// that is already a script_error, or has no code, comes back unchanged.
func AtLine(n int, err error) error {
	e, ok := errors.AsType[*Error](err)
	if !ok || e.Code == ScriptError {
		return err
	}
	return &Error{
		Code: ScriptError, Message: fmt.Sprintf("line %d: %s: %s", n, e.Code, e.Message),
		Hint: e.Hint, Arg: e.Arg, Line: n, Cause: e.Code, Details: e.Details,
	}
}

// CodeOf returns the code of the first *Error in err's chain, or "" if there
// is none.
func CodeOf(err error) Code {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Code
	}
	return ""
}
