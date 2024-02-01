package serviceidentity

import (
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

type ServiceIdentity struct {
	Name      string
	Namespace string
	// OwnerObject used to resolve the service name. May be nil if service name was resolved using annotation.
	OwnerObject client.Object
}

func (si *ServiceIdentity) GetFormattedOtterizeIdentity() string {
	return v1alpha3.GetFormattedOtterizeIdentity(si.Name, si.Namespace)
}

func (si *ServiceIdentity) GetName() string {
	return strings.Replace(si.Name, "svc.", "", 1)
}
