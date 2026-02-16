package errs

import "fmt"

// UserErrorf is a user-facing error.
// This helper exists mostly to avoid linters complaining about errors starting
// with a capitalized letter.
func UserErrorf(format string, a ...any) error {
	return fmt.Errorf(format, a...)
}

// Error wraps an underlying error with a user-facing reason.
//
// Reason is meant to be short and actionable; Err may contain technical details.
// When Err is nil, Error() falls back to Reason.
type Error struct {
	Err    error
	Reason string
}

// Wrap creates an Error with the given underlying error and user-facing reason.
func Wrap(err error, reason string) Error {
	return Error{Err: err, Reason: reason}
}

// Wrapf creates an Error with the given underlying error and a formatted reason.
func Wrapf(err error, format string, a ...any) Error {
	return Error{Err: err, Reason: fmt.Sprintf(format, a...)}
}

func (e Error) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Reason
}

func (e Error) Unwrap() error {
	return e.Err
}

// ReasonText returns the user-facing reason for the error.
func (e Error) ReasonText() string {
	return e.Reason
}
