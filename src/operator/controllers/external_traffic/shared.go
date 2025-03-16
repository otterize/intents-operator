package external_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
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

func GetIngressRefersToService(ctx context.Context, k8sClient client.Client, svc corev1.Service) (*v1.IngressList, error) {
	var ingressList v1.IngressList
	err := k8sClient.List(
		ctx,
		&ingressList,
		&client.MatchingFields{v2alpha1.IngressServiceNamesIndexField: svc.Name},
		&client.ListOptions{Namespace: svc.Namespace})
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return &ingressList, nil
}

func IsIngressListHasIPAWSALB(ingressList []v1.Ingress) bool {
	return lo.SomeBy(ingressList, func(ingress v1.Ingress) bool {
		if ingress.Annotations == nil {
			return false
		}

		targetType, ok := ingress.Annotations["alb.ingress.kubernetes.io/target-type"]
		if !ok {
			return false
		}

		return targetType == "ip"
	})
}

func IsServiceExternallyAccessible(svc corev1.Service, referringIngressList *v1.IngressList) bool {
	return svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeNodePort || len(referringIngressList.Items) > 0
}
