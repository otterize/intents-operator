package builders

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"testing"
)

type AllBuildersTestSuite struct {
	RulesBuilderTestSuiteBase
}

func (s *AllBuildersTestSuite) SetupTest() {
	s.RulesBuilderTestSuiteBase.SetupTest()
	s.Reconciler.AddIngressRuleBuilder(NewIngressNetpolBuilder())
	s.Reconciler.AddIngressRuleBuilder(NewPortNetworkPolicyReconciler(s.Client))
	s.Reconciler.AddEgressRuleBuilder(NewEgressNetworkPolicyBuilder())
	s.Reconciler.AddEgressRuleBuilder(NewPortEgressRulesBuilder(s.Client))
	s.Reconciler.AddEgressRuleBuilder(NewInternetEgressRulesBuilder())
}

func (s *AllBuildersTestSuite) TestCreateEveryRuleKind() {
	serviceName := "test-client"
	serverName := "test-server"
	serverNamespace := testServerNamespace
	formattedServer := (&serviceidentity.ServiceIdentity{Name: serverName, Namespace: serverNamespace}).GetFormattedOtterizeIdentity()
	formattedServerService := (&serviceidentity.ServiceIdentity{Name: serverName, Namespace: serverNamespace, Kind: serviceidentity.KindService}).GetFormattedOtterizeIdentity()
	formattedClient := (&serviceidentity.ServiceIdentity{Name: serviceName, Namespace: testNamespace}).GetFormattedOtterizeIdentity()
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      "client-intents",
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: fmt.Sprintf("%s.%s", serverName, serverNamespace),
			},
			{
				Name: fmt.Sprintf("svc:%s.%s", serverName, serverNamespace),
			},
			{
				Type:     otterizev1alpha3.IntentTypeInternet,
				Internet: &otterizev1alpha3.Internet{Ips: []string{"8.8.8.8/32"}},
			},
		},
	}

	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{{Spec: intentsSpec, ObjectMeta: metav1.ObjectMeta{Name: namespacedName.Name, Namespace: namespacedName.Namespace}}})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedNameClient := types.NamespacedName{
		Namespace: testNamespace,
		Name:      "test-client-access",
	}
	networkPolicyNamespacedNameServer := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      "test-server-access",
	}
	networkPolicyNamespacedNameServerService := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      "test-server-service-access",
	}

	// Expect GET for the client/server policy names - both return NotFound
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedNameClient, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			return apierrors.NewNotFound(v1.Resource("networkpolicy"), name.Name)
		})

	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedNameServer, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			return apierrors.NewNotFound(v1.Resource("networkpolicy"), name.Name)
		})

	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedNameServerService, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			return apierrors.NewNotFound(v1.Resource("networkpolicy"), name.Name)
		})

	serviceSelector := map[string]string{"app": "test-server"}
	svc := s.addExpectedKubernetesServiceCall("test-server", serverNamespace, 80, serviceSelector)

	egressRules := []v1.NetworkPolicyEgressRule{
		{To: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{otterizev1alpha3.OtterizeServiceLabelKey: formattedServer}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: serverNamespace}},
		}}},
		{Ports: []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 80}}}, To: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: serviceSelector},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: serverNamespace}},
		}}},
		{To: []v1.NetworkPolicyPeer{{IPBlock: &v1.IPBlock{CIDR: "8.8.8.8/32"}}}, Ports: make([]v1.NetworkPolicyPort, 0)}}

	egressNetpol := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameClient.Name,
			Namespace: networkPolicyNamespacedNameClient.Namespace,
			Labels:    map[string]string{otterizev1alpha3.OtterizeNetworkPolicy: formattedClient},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{otterizev1alpha3.OtterizeServiceLabelKey: formattedClient}},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			Ingress:     make([]v1.NetworkPolicyIngressRule, 0),
			Egress:      egressRules,
		},
	}
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&egressNetpol))
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), egressNetpol.Namespace, gomock.Any())

	ingressRulesNotSVC := []v1.NetworkPolicyIngressRule{
		{From: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{fmt.Sprintf(otterizev1alpha3.OtterizeAccessLabelKey, formattedServer): "true"}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: testNamespace}},
		}}}}

	netpolNotSVC := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameServer.Name,
			Namespace: networkPolicyNamespacedNameServer.Namespace,
			Labels:    map[string]string{otterizev1alpha3.OtterizeNetworkPolicy: formattedServer},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{otterizev1alpha3.OtterizeServiceLabelKey: formattedServer}},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			Ingress:     ingressRulesNotSVC,
			Egress:      make([]v1.NetworkPolicyEgressRule, 0),
		},
	}

	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&netpolNotSVC))
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), netpolNotSVC.Namespace, gomock.Any())

	ingressRulesToSVC := []v1.NetworkPolicyIngressRule{
		{Ports: []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 80}}}, From: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{fmt.Sprintf(otterizev1alpha3.OtterizeSvcAccessLabelKey, formattedServerService): "true"}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: testNamespace}},
		}}}}

	netpolToSVC := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameServerService.Name,
			Namespace: networkPolicyNamespacedNameServerService.Namespace,
			Labels:    map[string]string{otterizev1alpha3.OtterizeNetworkPolicy: formattedServerService},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			Ingress:     ingressRulesToSVC,
			Egress:      make([]v1.NetworkPolicyEgressRule, 0),
		},
	}

	err := controllerutil.SetOwnerReference(svc, &netpolToSVC, s.scheme)
	s.Require().NoError(err)
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&netpolToSVC))
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), netpolToSVC.Namespace, gomock.Any())

	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.ExpectEventsOrderAndCountDontMatter(consts.ReasonCreatedEgressNetworkPolicies, consts.ReasonCreatedNetworkPolicies)
	s.NoError(err)
	s.Empty(res)
}

func TestAllBuildersTestSuite(t *testing.T) {
	suite.Run(t, new(AllBuildersTestSuite))
}
