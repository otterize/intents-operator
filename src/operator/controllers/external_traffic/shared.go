package external_traffic

import (
	"k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func serviceNamesFromIngress(ingress *v1.Ingress) sets.String {
	serviceNames := sets.NewString()
	if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
		serviceNames.Insert(ingress.Spec.DefaultBackend.Service.Name)
	}

	for _, rule := range ingress.Spec.Rules {
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service != nil {
				serviceNames.Insert(path.Backend.Service.Name)
			}
		}
	}

	return serviceNames
}
