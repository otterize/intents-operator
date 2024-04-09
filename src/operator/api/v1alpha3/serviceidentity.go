package v1alpha3

import "github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"

func (in *ClientIntents) ToServiceIdentity() *serviceidentity.ServiceIdentity {
	return &serviceidentity.ServiceIdentity{
		Name:      in.Spec.Service.Name,
		Namespace: in.Namespace,
		Group:     in.Spec.Service.Group,
		Kind:      in.GetClientKind(),
	}
}

func (in *Intent) ToServiceIdentity(intentsObjNamespace string) *serviceidentity.ServiceIdentity {
	return &serviceidentity.ServiceIdentity{
		Name:      in.GetTargetServerName(),
		Namespace: in.GetTargetServerNamespace(intentsObjNamespace),
		Kind:      in.GetTargetServerKind(),
		Group:     in.GetTargetServerGroup(),
	}
}

func (in *ProtectedService) ToServiceIdentity() *serviceidentity.ServiceIdentity {
	return &serviceidentity.ServiceIdentity{
		Name:      in.Spec.Name,
		Namespace: in.Namespace,
		Group:     in.Spec.Group,
		Kind:      in.GetKind(),
	}
}
