package intents_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
		&client.MatchingFields{otterizev1alpha2.OtterizeProtectedServiceNameIndexField: serverName},
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
