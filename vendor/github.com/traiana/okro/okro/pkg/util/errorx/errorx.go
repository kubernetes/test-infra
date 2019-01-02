package errorx

import (
	"errors"
	"fmt"
)

// New returns an error that formats as the given text.
func New(text string) error {
	return errors.New(text)
}

// Newf formats according to a format specifier and returns the string
// as a value that satisfies error.
func Newf(format string, a ...interface{}) error {
	return fmt.Errorf(format, a...)
}

// Deep is a shorthand for creating a nested single-error structure.
// For multiple errors, Errors should be created directly.
func Deep(err error, path ...interface{}) Errors {
	es := Errors{}
	es.Insert(err, path)
	return es
}

// InvalidArg returns an error including the invalid value.
func InvalidArg(v interface{}) error {
	return Newf("invalid argument: %#v", v)
}
