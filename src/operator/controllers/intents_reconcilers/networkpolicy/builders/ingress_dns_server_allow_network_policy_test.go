package builders

import (
	"context"
	"fmt"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type DNSServerBuilderNetworkPolicyReconcilerTestSuite struct {
	RulesBuilderTestSuiteBase
	Builder *IngressDNSServerAutoAllowNetpolBuilder
}

func (s *DNSServerBuilderNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.RulesBuilderTestSuiteBase.SetupTest()
	s.Builder = NewIngressDNSServerAutoAllowNetpolBuilder()
	s.Builder.Recorder = s.Recorder
	s.Reconciler.AddIngressRuleBuilder(s.Builder)
}

func (s *DNSServerBuilderNetworkPolicyReconcilerTestSuite) TearDownTest() {
	s.RulesBuilderTestSuiteBase.TearDownTest()
	s.Builder = nil
}

func (s *DNSServerBuilderNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "coredns-access"
	serviceName := "test-client"
	serverNamespace := "kube-system"
	formattedTargetServer := "coredns-kube-system-dd756f"

	s.Reconciler.EnforcementDefaultState = true
	namespacedName := types.NamespacedName{
		Namespace: "kube-system",
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}
	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: serviceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: fmt.Sprintf("coredns.%s", serverNamespace)},
			},
		},
	}

	s.expectGetAllEffectivePolicies([]otterizev2alpha1.ClientIntents{{Spec: intentsSpec, ObjectMeta: metav1.ObjectMeta{Name: namespacedName.Name, Namespace: namespacedName.Namespace}}})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			return apierrors.NewNotFound(v1.Resource("networkpolicy"), name.Name)
		})

	// Create NetworkPolicy
	newPolicy := ingressDNSnetworkPolicyIngressTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	selector := labels.SelectorFromSet(labels.Set(map[string]string{
		otterizev2alpha1.OtterizeServiceLabelKey: formattedTargetServer,
	}))

	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), serverNamespace, selector)
	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func ingressDNSnetworkPolicyIngressTemplate(
	policyName string,
	targetNamespace string,
	formattedTargetServer string,
	intentsObjNamespaces ...string,
) *v1.NetworkPolicy {
	ingressRules := lo.Map(intentsObjNamespaces, func(namespace string, _ int) v1.NetworkPolicyIngressRule {
		return v1.NetworkPolicyIngressRule{
			Ports: []v1.NetworkPolicyPort{{
				Protocol: lo.ToPtr(v12.ProtocolUDP),
				Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 53},
			}},
		}
	})
	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeNetworkPolicy: formattedTargetServer,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev2alpha1.OtterizeServiceLabelKey: formattedTargetServer,
				},
			},
			Ingress: ingressRules,
		},
	}
}

func TestDNSServerBuilderNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(DNSServerBuilderNetworkPolicyReconcilerTestSuite))
}
