package v2

import "github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"

func (in *ClientIntents) ToServiceIdentity() serviceidentity.ServiceIdentity {
	return serviceidentity.ServiceIdentity{
		Name:      in.Spec.Workload.Name,
		Namespace: in.Namespace,
		Kind:      in.GetClientKind(),
	}
}

func (in *Target) ToServiceIdentity(intentsObjNamespace string) serviceidentity.ServiceIdentity {
	return serviceidentity.ServiceIdentity{
		Name:      in.GetTargetServerName(),
		Namespace: in.GetTargetServerNamespace(intentsObjNamespace),
		Kind:      in.GetTargetServerKind(),
	}
}

func (in *ProtectedService) ToServiceIdentity() serviceidentity.ServiceIdentity {
	return serviceidentity.ServiceIdentity{
		Name:      in.Spec.Name,
		Namespace: in.Namespace,
		Kind:      in.GetKind(),
	}
}
