package builders

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
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
	"k8s.io/apimachinery/pkg/util/intstr"
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

	portToProtocol := make(map[int]corev1.Protocol)
	for _, port := range svc.Spec.Ports {
		if port.TargetPort.StrVal != "" {
			continue
		}
		portToProtocol[port.TargetPort.IntValue()] = port.Protocol
	}

	networkPolicyPorts := make([]v1.NetworkPolicyPort, 0)
	// Create a list of network policy ports
	for port, protocol := range portToProtocol {
		netpolPort := v1.NetworkPolicyPort{
			Port: &intstr.IntOrString{IntVal: int32(port)},
		}
		if len(protocol) != 0 {
			netpolPort.Protocol = lo.ToPtr(protocol)
		}
		networkPolicyPorts = append(networkPolicyPorts, netpolPort)
	}

	for _, clientCall := range ep.CalledBy {
		if clientCall.IntendedCall.Type != "" && clientCall.IntendedCall.Type != otterizev1alpha3.IntentTypeHTTP && clientCall.IntendedCall.Type != otterizev1alpha3.IntentTypeKafka {
			continue
		}
		if clientCall.IntendedCall.IsTargetTheKubernetesAPIServer(ep.Service.Namespace) {
			// Currently only egress is supported for the kubernetes API server
			continue
		}
		// create only one ingress run for each namespace
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
								otterizev1alpha3.OtterizeSvcAccessLabelKey, ep.Service.GetFormattedOtterizeIdentity()): "true",
						},
					},
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: clientCall.Service.Namespace,
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
