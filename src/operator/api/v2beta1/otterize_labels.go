package v2beta1

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"golang.org/x/exp/maps"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

// IsMissingOtterizeAccessLabels checks if a pod's labels need updating
func IsMissingOtterizeAccessLabels(pod *v1.Pod, otterizeAccessLabels map[string]string) bool {
	podOtterizeAccessLabels := GetOtterizeLabelsFromPod(pod)
	if len(podOtterizeAccessLabels) != len(otterizeAccessLabels) {
		return true
	}

	// Length is equal, check for diff in keys
	for k := range podOtterizeAccessLabels {
		if _, ok := otterizeAccessLabels[k]; !ok {
			return true
		}
	}
	return false
}

// UpdateOtterizeAccessLabels updates a pod's labels with Otterize labels representing their intents
// The pod is also labeled with "otterize-client=<hashed-client-name>" to mark it as having intents or being the client-side of an egress netpol
func UpdateOtterizeAccessLabels(pod *v1.Pod, serviceIdentity serviceidentity.ServiceIdentity, otterizeAccessLabels map[string]string) *v1.Pod {
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	// TODO: We should understand what kind
	pod = cleanupOtterizeLabelsAndAnnotations(pod)
	for k, v := range otterizeAccessLabels {
		pod.Labels[k] = v
	}
	pod.Labels[OtterizeClientLabelKey] = serviceIdentity.GetFormattedOtterizeIdentityWithoutKind()
	return pod
}

func HasOtterizeServiceLabel(pod *v1.Pod, labelValue string) bool {
	value, exists := pod.Labels[OtterizeServiceLabelKey]
	return exists && value == labelValue
}

func HasOtterizeOwnerKindLabel(pod *v1.Pod, labelValue string) bool {
	value, exists := pod.Labels[OtterizeOwnerKindLabelKey]
	return exists && value == labelValue
}

func HasOtterizeDeprecatedServerLabel(pod *v1.Pod) bool {
	_, exists := pod.Labels[OtterizeServerLabelKeyDeprecated]
	return exists
}

func cleanupOtterizeLabelsAndAnnotations(pod *v1.Pod) *v1.Pod {
	for k := range pod.Labels {
		if isOtterizeAccessLabel(k) {
			delete(pod.Labels, k)
		}
	}

	delete(pod.Annotations, AllIntentsRemovedAnnotation)

	return pod
}

func isOtterizeAccessLabel(s string) bool {
	return strings.HasPrefix(s, OtterizeAccessLabelPrefix) || strings.HasPrefix(s, OtterizeServiceAccessLabelPrefix)
}

func CleanupOtterizeKubernetesServiceLabels(pod *v1.Pod) *v1.Pod {
	for k := range pod.Labels {
		if isOtterizeKubernetesServiceLabel(k) {
			delete(pod.Labels, k)
		}
	}

	return pod
}

func isOtterizeKubernetesServiceLabel(s string) bool {
	return strings.HasPrefix(s, OtterizeKubernetesServiceLabelKeyPrefix)
}

func GetOtterizeLabelsFromPod(pod *v1.Pod) map[string]string {
	otterizeLabels := make(map[string]string)
	for k, v := range pod.Labels {
		if isOtterizeAccessLabel(k) {
			otterizeLabels[k] = v
		}
	}

	return otterizeLabels
}

func ServiceIdentityToLabelsForWorkloadSelection(ctx context.Context, k8sClient client.Client, identity serviceidentity.ServiceIdentity) (map[string]string, bool, error) {
	// This is here for backwards compatibility
	if identity.Kind == "" || identity.Kind == serviceidentity.KindOtterizeLegacy {
		return map[string]string{OtterizeServiceLabelKey: identity.GetFormattedOtterizeIdentityWithoutKind()}, true, nil
	}

	if identity.Kind == serviceidentity.KindService {
		svc := v1.Service{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: identity.Name, Namespace: identity.Namespace}, &svc)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil, false, nil
			}
			return nil, false, errors.Wrap(err)
		}
		if svc.Spec.Selector == nil {
			return nil, false, fmt.Errorf("service %s/%s has no selector", svc.Namespace, svc.Name)
		}
		return maps.Clone(svc.Spec.Selector), true, nil
	}

	// This should be replaced with a logic that gets the pod owners and uses its labelsSelector (for known kinds)
	return map[string]string{OtterizeOwnerKindLabelKey: identity.Kind,
		OtterizeServiceLabelKey: identity.GetFormattedOtterizeIdentityWithoutKind()}, true, nil
}
