package errorreporter

import (
	"fmt"
	bugsnagerrors "github.com/bugsnag/bugsnag-go/v2/errors"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
)

// Based on github.com/Shopify/logrus-bugsnag, but adapted to use the v2 version of bugsnag-go
type errorReportingHook struct{}

// NewErrorReportingHook initializes a logrus hook which sends exceptions to an
// exception-tracking service compatible with the error telemetry API. Before using
// this hook, you must call initSender(). The returned object should be
// registered with a log via `AddHook()`
//
// Entries that trigger an Error, Fatal or Panic should now include an "error"
// field to send to Bugsnag.
func NewErrorReportingHook() (*errorReportingHook, error) {
	return &errorReportingHook{}, nil
}

// skipStackFrames skips logrus stack frames before logging to the error telemetry reporting API.
const skipStackFrames = 3
const errorLogKey = "error"

type ErrorList interface {
	Unwrap() []error
}

// Fire forwards an error to error telemetry reporting API. Given a logrus.Entry, it extracts the
// "error" field (or the Message if the error isn't present) and sends it off.
func (hook *errorReportingHook) Fire(entry *logrus.Entry) error {
	return sendToErrorTelemetry(entry, false)
}

func getErrors(entry *logrus.Entry) []error {
	errFromMessage := bugsnagerrors.New(entry.Message, 1).Err
	errFromLog, ok := entry.Data[errorLogKey].(error)
	if !ok {
		return []error{errFromMessage}
	}
	// don't delete error key from entry as it's a standard logrus field and other hooks may make use of it

	bugsnagErr, ok := errFromLog.(*bugsnagerrors.Error)
	if !ok {
		return []error{errFromLog}
	}

	// Check if the underlying error field contains a list, usually created by calling to go stdlib errors.Join
	errList, ok := bugsnagErr.Err.(ErrorList)
	if !ok {
		return []error{bugsnagErr}
	}
	return errList.Unwrap()
}
func sendToErrorTelemetry(entry *logrus.Entry, sync bool) error {
	notifyErrs := getErrors(entry)

	metadata := make(map[string]string)
	for key, val := range entry.Data {
		if key != "error" {
			metadata[key] = fmt.Sprintf("%v", val)
		}
	}

	for _, notifyErr := range notifyErrs {
		errWithStack := errors.WrapWithSkip(notifyErr, skipStackFrames)
		if sync {
			return sendErrorSync(errWithStack.(*bugsnagerrors.Error), metadata)
		}
		sendErrorAsync(errWithStack.(*bugsnagerrors.Error), metadata)
	}

	return nil
}

// Levels enumerates the log levels on which the error should be forwarded to
// bugsnag: everything at or above the "Error" level.
func (hook *errorReportingHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.ErrorLevel,
		logrus.FatalLevel,
		logrus.PanicLevel,
	}
}
