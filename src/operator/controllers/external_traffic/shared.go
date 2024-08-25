package external_traffic

import (
	"github.com/samber/lo"
	"k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/sets"
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

func isIngressListHasInternetFacingAWSALB(ingressList []v1.Ingress) bool {
	return lo.SomeBy(ingressList, func(ingress v1.Ingress) bool {
		if ingress.Annotations == nil {
			return false
		}

		scheme, ok := ingress.Annotations["alb.ingress.kubernetes.io/scheme"]
		if !ok {
			return false
		}

		return scheme == "internet-facing"
	})
}
