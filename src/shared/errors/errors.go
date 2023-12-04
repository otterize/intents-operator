package errors

import (
	gerrors "errors"
	"fmt"
	bugsnagerrors "github.com/bugsnag/bugsnag-go/v2/errors"
)

func New(text string) error {
	return bugsnagerrors.New(text, 1)
}

func Errorf(format string, a ...any) error {
	return bugsnagerrors.New(fmt.Errorf(format, a...), 1)
}

func ErrorfWithSkip(skip int, format string, a ...any) error {
	return bugsnagerrors.New(fmt.Errorf(format, a...), 1+skip)
}

func As(err error, target interface{}) bool {
	// This handles the case where another error wraps our *bugsnagerrors.Error. We try to call `As`
	// at each step, and recursively unwrap until we can no longer do it. Similar to the original `As` logic.
	// See test TestThirdpartyWrapOnOurWrap
	for err != nil {
		if bugsnagErr, ok := err.(*bugsnagerrors.Error); ok {
			if gerrors.As(bugsnagErr.Err, target) {
				return true
			}
		}
		if gerrors.As(err, target) {
			return true
		}
		err = Unwrap(err)
	}

	return false
}

// Is detects whether the error is equal to a given error. Errors
// are considered equal by this function if they are matched by errors.Is
// or if their contained errors are matched through errors.Is
func Is(e error, original error) bool {
	if gerrors.Is(e, original) {
		return true
	}

	if bugsnagErr, ok := e.(*bugsnagerrors.Error); ok {
		return Is(bugsnagErr.Err, original)
	}

	if original, ok := original.(*bugsnagerrors.Error); ok {
		return Is(e, original.Err)
	}

	return false
}

func Unwrap(err error) error {
	if bugsnagErr, ok := err.(*bugsnagerrors.Error); ok {
		return bugsnagErr.Err
	}
	return gerrors.Unwrap(err)
}

func wrapImpl(err error, skip int) error {
	if err == nil {
		return nil
	}

	return bugsnagerrors.New(err, skip+1)
}

func Wrap(err error) error {
	return wrapImpl(err, 1)
}

func WrapWithSkip(err error, skip int) error {
	return wrapImpl(err, skip+1)
}
