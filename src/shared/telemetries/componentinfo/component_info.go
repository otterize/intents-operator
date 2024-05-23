package componentinfo

import (
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"sync"
)

var (
	globalContextId               string
	globalComponentInstanceId     string
	globalComponentInstanceIdOnce sync.Once
)

func SetGlobalContextId(contextId string) {
	globalContextId = contextId
}

func GlobalContextId() string {
	if globalContextId == "" {
		logrus.Panic("context ID not set")
	}
	return globalContextId
}

func GlobalComponentInstanceId() string {
	globalComponentInstanceIdOnce.Do(func() {
		globalComponentInstanceId = uuid.NewString()
	})
	return globalComponentInstanceId
}
