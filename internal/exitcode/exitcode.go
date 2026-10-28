// Package exitcode defines the process exit contract shared by every command.
//
// The contract is intentionally strict so scripts and agents can branch on the
// exit code instead of parsing stderr:
//
//	0  success
//	1  business failure (API envelope code != 0)
//	2  usage or configuration error (nothing was sent)
//	3  runtime failure (network, timeout, 5xx, malformed response)
//
// The API envelope is still written to stdout for 1, so `... | jq .code`
// keeps working.
package exitcode

import (
	"errors"
	"fmt"
)

const (
	OK       = 0
	Business = 1
	Usage    = 2
	Runtime  = 3
)

// Error wraps an error with the exit code the process should use.
type Error struct {
	Code int
	Err  error
	// Quiet suppresses the "Error: ..." line on stderr. Business failures use
	// it because the envelope on stdout is already the machine-readable error.
	Quiet bool
}

func (e *Error) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit %d", e.Code)
	}
	return e.Err.Error()
}

func (e *Error) Unwrap() error { return e.Err }

// New builds an exit-code error.
func New(code int, err error) error {
	if err == nil {
		err = errors.New("command failed")
	}
	return &Error{Code: code, Err: err}
}

// Usagef reports a caller mistake: bad flag, missing required field, bad JSON.
func Usagef(format string, args ...any) error {
	return &Error{Code: Usage, Err: fmt.Errorf(format, args...)}
}

// Runtimef reports an environmental failure: network, timeout, 5xx.
func Runtimef(format string, args ...any) error {
	return &Error{Code: Runtime, Err: fmt.Errorf(format, args...)}
}

// Businessf reports an API-level failure that has already been rendered.
func Businessf(format string, args ...any) error {
	return &Error{Code: Business, Err: fmt.Errorf(format, args...), Quiet: true}
}

// CodeOf returns the exit code carried by err, or Runtime for unknown errors.
func CodeOf(err error) int {
	if err == nil {
		return OK
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return Runtime
}

// IsQuiet reports whether the error should be printed to stderr.
func IsQuiet(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Quiet
}
