package shared

import (
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

func RegisterPanicHandlers() {
	utilruntime.PanicHandlers = []func(interface{}){
		panicHandler,
	}
}

// shared.panicHandler
// controller.Reconciler.recover
// runtime.gopanic
// original panic location <--
const skipStackFramesCount = 3

func panicHandler(item any) {
	err := errors.ErrorfWithSkip(skipStackFramesCount, "panic: %v", item)

	if errOrig, ok := item.(error); ok {
		err = errors.WrapWithSkip(errOrig, skipStackFramesCount)
	}

	logrus.WithError(err).Error("caught panic")
}
