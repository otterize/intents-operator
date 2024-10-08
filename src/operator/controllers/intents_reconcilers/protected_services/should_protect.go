package protected_services

import (
	"context"
	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IsServerEnforcementEnabledDueToProtectionOrDefaultState(
	ctx context.Context,
	kube client.Client,
	serverServiceId serviceidentity.ServiceIdentity,
	enforcementDefaultState bool,
	activeNamespaces *goset.Set[string],
) (bool, error) {
	if enforcementDefaultState {
		logrus.Debug("Enforcement is default on, so all services should be protected")
		return true, nil
	}
	logrus.Debug("Protected services are enabled")

	logrus.Debugf("checking if server's namespace is in acrive namespaces")
	if activeNamespaces != nil && activeNamespaces.Contains(serverServiceId.Namespace) {
		logrus.Debugf("Server %s in namespace %s is in active namespaces", serverServiceId.Name, serverServiceId.Namespace)
		return true, nil
	}

	logrus.Debugf("checking if server is in protected list")
	var protectedServicesResources otterizev2alpha1.ProtectedServiceList
	err := kube.List(ctx, &protectedServicesResources,
		client.MatchingFields{otterizev2alpha1.OtterizeProtectedServiceNameIndexField: serverServiceId.Name},
		client.InNamespace(serverServiceId.Namespace))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrap(err)
	}

	// we need to find at least one protected service to have the same kind (or no kind) as the serviceIdentity to enforce
	for _, ps := range protectedServicesResources.Items {
		if serverServiceId.Kind != "" && ps.Spec.Kind != "" && ps.Spec.Kind != serverServiceId.Kind {
			continue
		}
		logrus.Debugf("Server %s in namespace %s is in protected list", serverServiceId.Name, serverServiceId.Namespace)
		return true, nil
	}

	logrus.Debugf("Server %s in namespace %s is not in protected list", serverServiceId.Name, serverServiceId.Namespace)
	return false, nil
}

// InitProtectedServiceIndexField indexes protected service resources by their service name
// This is used in finalizers to determine whether a network policy should be removed from the target namespace
func InitProtectedServiceIndexField(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&otterizev2alpha1.ProtectedService{},
		otterizev2alpha1.OtterizeProtectedServiceNameIndexField,
		func(object client.Object) []string {
			protectedService := object.(*otterizev2alpha1.ProtectedService)
			if protectedService.Spec.Name == "" {
				return nil
			}

			return []string{protectedService.Spec.Name}
		})
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}
