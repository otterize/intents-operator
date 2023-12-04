package componentinfo

import "github.com/google/uuid"

var (
	globalContextId           string
	globalComponentInstanceId string
	globalVersion             string
	globalCloudClientId       string
)

func SetGlobalContextId(contextId string) {
	globalContextId = contextId
}

func SetGlobalVersion(version string) {
	globalVersion = version
}

func SetGlobalCloudClientId(clientId string) {
	globalCloudClientId = clientId
}

func SetGlobalComponentInstanceId() {
	globalComponentInstanceId = uuid.NewString()
}

func GlobalContextId() string {
	return globalContextId
}

func GlobalComponentInstanceId() string {
	return globalComponentInstanceId
}

func GlobalVersion() string {
	return globalVersion
}

func GlobalCloudClientId() string {
	return globalCloudClientId
}
