package serviceidentity

import (
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/samber/lo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

type ServiceIdentity struct {
	Name      string
	Namespace string
	Kind      string
	// OwnerObject used to resolve the service name. May be nil if service name was resolved using annotation.
	OwnerObject client.Object
}

const KindService = "Service"

func (si *ServiceIdentity) GetFormattedOtterizeIdentity() string {
	if si.Kind == KindService {
		return v1alpha3.GetFormattedOtterizeIdentity("svc."+si.Name, si.Namespace)
	}
	return v1alpha3.GetFormattedOtterizeIdentity(si.Name, si.Namespace)
}

func (si *ServiceIdentity) GetName() string {
	return si.Name
}

func (si *ServiceIdentity) GetNameWithKind() string {
	return lo.Ternary(si.Kind == "", si.Name, fmt.Sprintf("%s-%s", si.Name, strings.ToLower(si.Kind)))
}

func NewFromIntent(intent v1alpha3.Intent, clientNamespace string) ServiceIdentity {
	return ServiceIdentity{
		Name:      intent.GetTargetServerName(),
		Namespace: intent.GetTargetServerNamespace(clientNamespace),
		Kind:      lo.Ternary(intent.IsTargetServerKubernetesService(), KindService, ""),
	}
}

func NewFromClientIntent(clientIntent v1alpha3.ClientIntents) ServiceIdentity {
	return ServiceIdentity{
		Name:      clientIntent.Spec.Service.Name,
		Namespace: clientIntent.Namespace,
	}
}
