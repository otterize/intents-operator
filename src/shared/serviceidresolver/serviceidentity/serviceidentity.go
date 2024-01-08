package serviceidentity

import "sigs.k8s.io/controller-runtime/pkg/client"

type ServiceIdentity struct {
	Name      string
	Namespace string
	// OwnerObject used to resolve the service name. May be nil if service name was resolved using annotation.
	OwnerObject client.Object
}
