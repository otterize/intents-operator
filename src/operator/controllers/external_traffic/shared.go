package external_traffic

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func serviceNamesFromIngress(ingress *v1.Ingress) sets.Set[string] {
	serviceNames := sets.Set[string]{}
	if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
		serviceNames.Insert(ingress.Spec.DefaultBackend.Service.Name)
	}

	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil {
					serviceNames.Insert(path.Backend.Service.Name)
				}
			}
		}
	}

	return serviceNames
}

func isServiceExternallyAccessible(svc *corev1.Service, referringIngressList *v1.IngressList) bool {
	return svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeNodePort || len(referringIngressList.Items) > 0
}

func convertToCloudExternalService(svc *corev1.Service, referringIngressList *v1.IngressList) (graphqlclient.ExternallyAccessibleServiceInput, bool, error) {
	if !isServiceExternallyAccessible(svc, referringIngressList) {
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, nil
	}

	ReferredByIngress := len(referringIngressList.Items) > 0
	var cloudServiceType graphqlclient.KubernetesServiceType
	switch svc.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		cloudServiceType = graphqlclient.KubernetesServiceTypeLoadBalancer
	case corev1.ServiceTypeNodePort:
		cloudServiceType = graphqlclient.KubernetesServiceTypeNodePort
	case corev1.ServiceTypeClusterIP:
		cloudServiceType = graphqlclient.KubernetesServiceTypeClusterIp
	case corev1.ServiceTypeExternalName:
		cloudServiceType = graphqlclient.KubernetesServiceTypeExternalName
	default:
		// Should not reach here until k8s adds new service type
		return graphqlclient.ExternallyAccessibleServiceInput{}, false, errors.New(fmt.Sprintf("unsupported service type: %s", svc.Spec.Type))
	}

	serviceInput := graphqlclient.ExternallyAccessibleServiceInput{
		Namespace:         svc.Namespace,
		ServerName:        svc.Name,
		ReferredByIngress: ReferredByIngress,
		ServiceType:       cloudServiceType,
	}
	return serviceInput, true, nil
}

func getIngressRefersToService(ctx context.Context, k8sClient client.Client, svc *corev1.Service) (*v1.IngressList, error) {
	var ingressList v1.IngressList
	err := k8sClient.List(
		ctx,
		&ingressList,
		&client.MatchingFields{v1alpha3.IngressServiceNamesIndexField: svc.Name},
		&client.ListOptions{Namespace: svc.Namespace})
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return &ingressList, nil
}
