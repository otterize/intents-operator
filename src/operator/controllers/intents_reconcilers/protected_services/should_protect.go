package protected_services

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ShouldCreateNetworkPoliciesDueToProtectionOrDefaultState(ctx context.Context, kube client.Client, serverName string, serverNamespace string, enforcementDefaultState bool) (bool, error) {
	if enforcementDefaultState {
		logrus.Debug("Enforcement is default on, so all services should be protected")
		return true, nil
	}

	logrus.Debug("Protected services are enabled, checking if server is in protected list")
	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	err := kube.List(ctx, &protectedServicesResources,
		client.MatchingFields{otterizev1alpha2.OtterizeProtectedServiceNameIndexField: serverName},
		client.InNamespace(serverNamespace))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
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
		&otterizev1alpha2.ProtectedService{},
		otterizev1alpha2.OtterizeProtectedServiceNameIndexField,
		func(object client.Object) []string {
			protectedService := object.(*otterizev1alpha2.ProtectedService)
			if protectedService.Spec.Name == "" {
				return nil
			}

			return []string{protectedService.Spec.Name}
		})
	if err != nil {
		return err
	}

	return nil
}
