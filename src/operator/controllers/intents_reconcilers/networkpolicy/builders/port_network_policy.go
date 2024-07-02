package builders

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PortNetworkPolicyReconciler struct {
	client.Client
	injectablerecorder.InjectableRecorder
}

func NewPortNetworkPolicyReconciler(
	c client.Client,
) *PortNetworkPolicyReconciler {
	return &PortNetworkPolicyReconciler{
		Client: c,
	}
}

func (r *PortNetworkPolicyReconciler) buildIngressRulesFromEffectivePolicy(ep effectivepolicy.ServiceEffectivePolicy, svc *corev1.Service) []v1.NetworkPolicyIngressRule {
	ingressRules := make([]v1.NetworkPolicyIngressRule, 0)
	fromNamespaces := goset.NewSet[string]()

	// Create a list of network policy ports
	networkPolicyPorts := make([]v1.NetworkPolicyPort, 0)
	for _, port := range svc.Spec.Ports {
		netpolPort := v1.NetworkPolicyPort{
			Port: lo.ToPtr(port.TargetPort),
		}
		if len(port.Protocol) != 0 {
			netpolPort.Protocol = lo.ToPtr(port.Protocol)
		}
		networkPolicyPorts = append(networkPolicyPorts, netpolPort)
	}

	for _, clientCall := range ep.CalledBy {
		if clientCall.IntendedCall.IsTargetOutOfCluster() {
			continue
		}
		if clientCall.IntendedCall.IsTargetTheKubernetesAPIServer(ep.Service.Namespace) {
			// Currently only egress is supported for the kubernetes API server
			continue
		}
		// We use the same from.podSelector for every namespace,
		// therefore there is no need for more than one ingress rule per namespace
		if fromNamespaces.Contains(clientCall.Service.Namespace) {
			continue
		}
		ingressRule := v1.NetworkPolicyIngressRule{
			Ports: networkPolicyPorts,
			From: []v1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							fmt.Sprintf(
								otterizev2alpha1.OtterizeSvcAccessLabelKey, ep.Service.GetFormattedOtterizeIdentityWithKind()): "true",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							otterizev2alpha1.KubernetesStandardNamespaceNameLabelKey: clientCall.Service.Namespace,
						},
					},
				},
			},
		}
		ingressRules = append(ingressRules, ingressRule)
		fromNamespaces.Add(clientCall.Service.Namespace)
	}
	return ingressRules
}

func (r *PortNetworkPolicyReconciler) Build(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy) ([]v1.NetworkPolicyIngressRule, error) {
	if ep.Service.Kind != serviceidentity.KindService {
		return make([]v1.NetworkPolicyIngressRule, 0), nil
	}
	svc := corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: ep.Service.Name, Namespace: ep.Service.Namespace}, &svc)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return make([]v1.NetworkPolicyIngressRule, 0), nil
		}
		return nil, errors.Wrap(err)
	}
	if svc.Spec.Selector == nil {
		return nil, errors.Errorf("service %s/%s has no selector", svc.Namespace, svc.Name)
	}
	return r.buildIngressRulesFromEffectivePolicy(ep, &svc), nil
}
