package health

import (
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"
)

var lastReconcileStartTime = time.Time{}
var lastReconcileEndTime = time.Time{}

func UpdateLastReconcileStartTime() {
	lastReconcileStartTime = time.Now()
}

func UpdateLastReconcileEndTime() {
	lastReconcileEndTime = time.Now()
}

func ElapsedTimeSinceReconcileStartWithoutSuccessfulReconcile() time.Duration {
	if lastReconcileStartTime.IsZero() {
		return time.Duration(0)
	}

	// Successful reconcile
	if lastReconcileEndTime.After(lastReconcileStartTime) {
		return time.Duration(0)
	}

	return time.Since(lastReconcileStartTime)
}

func Checker(*http.Request) error {
	if ElapsedTimeSinceReconcileStartWithoutSuccessfulReconcile() > 30*time.Second {
		err := errors.Errorf("last reconcile took more than 5 minutes - failing healthcheck")
		logrus.WithError(err).Error("Health check failed due to long reconcile time - may be normal if just enabled enforcement for the first time on a large cluster, if it recovers")
		return err
	}

	return nil
}
