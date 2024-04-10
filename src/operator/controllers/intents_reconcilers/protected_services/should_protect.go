package protected_services

import (
	"context"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IsServerEnforcementEnabledDueToProtectionOrDefaultState(ctx context.Context, kube client.Client, serverName string, serverNamespace string, enforcementDefaultState bool, activeNamespaces *goset.Set[string]) (bool, error) {
	if enforcementDefaultState {
		logrus.Debug("Enforcement is default on, so all services should be protected")
		return true, nil
	}
	logrus.Debug("Protected services are enabled")

	logrus.Debugf("checking if server's namespace is in acrive namespaces")
	if activeNamespaces != nil && activeNamespaces.Contains(serverNamespace) {
		logrus.Debugf("Server %s in namespace %s is in active namespaces", serverName, serverNamespace)
		return true, nil
	}

	logrus.Debugf("checking if server is in protected list")
	var protectedServicesResources otterizev1alpha3.ProtectedServiceList
	err := kube.List(ctx, &protectedServicesResources,
		client.MatchingFields{otterizev1alpha3.OtterizeProtectedServiceNameIndexField: serverName},
		client.InNamespace(serverNamespace))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrap(err)
	}

	if len(protectedServicesResources.Items) != 0 {
		logrus.Debugf("Server %s in namespace %s is in protected list", serverName, serverNamespace)
		return true, nil
	}

	logrus.Debugf("Server %s in namespace %s is not in protected list", serverName, serverNamespace)
	return false, nil
}

// InitProtectedServiceIndexField indexes protected service resources by their service name
// This is used in finalizers to determine whether a network policy should be removed from the target namespace
func InitProtectedServiceIndexField(mgr ctrl.Manager) error {
	err := mgr.GetCache().IndexField(
		context.Background(),
		&otterizev1alpha3.ProtectedService{},
		otterizev1alpha3.OtterizeProtectedServiceNameIndexField,
		func(object client.Object) []string {
			protectedService := object.(*otterizev1alpha3.ProtectedService)
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
