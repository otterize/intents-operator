package builders

import (
	"context"
	"fmt"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
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
	formattedServer := (&serviceidentity.ServiceIdentity{Name: serverName, Namespace: serverNamespace}).GetFormattedOtterizeIdentityWithoutKind()
	formattedServerService := (&serviceidentity.ServiceIdentity{Name: serverName, Namespace: serverNamespace, Kind: serviceidentity.KindService}).GetFormattedOtterizeIdentityWithKind()
	formattedClient := (&serviceidentity.ServiceIdentity{Name: serviceName, Namespace: testNamespace}).GetFormattedOtterizeIdentityWithoutKind()
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      "client-intents",
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}
	intentsSpec := &otterizev2.IntentsSpec{
		Workload: otterizev2.Workload{Name: serviceName},
		Targets: []otterizev2.Target{
			{
				Kubernetes: &otterizev2.KubernetesTarget{Name: fmt.Sprintf("%s.%s", serverName, serverNamespace)},
			},
			{
				Kubernetes: &otterizev2.KubernetesTarget{Name: fmt.Sprintf("%s.%s", serverName, serverNamespace), Kind: serviceidentity.KindService},
			},
			{
				Internet: &otterizev2.Internet{Ips: []string{"8.8.8.8/32"}},
			},
		},
	}

	s.expectGetAllEffectivePolicies([]otterizev2.ClientIntents{{Spec: intentsSpec, ObjectMeta: metav1.ObjectMeta{Name: namespacedName.Name, Namespace: namespacedName.Namespace}}})

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
	svc := s.addExpectedKubernetesServiceCall("test-server", serverNamespace, []corev1.ServicePort{{TargetPort: intstr.IntOrString{IntVal: 80}}}, serviceSelector)

	egressRules := []v1.NetworkPolicyEgressRule{
		{To: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.OtterizeServiceLabelKey: formattedServer}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: serverNamespace}},
		}}},
		{To: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{testServerServiceLabelKey: testServerServiceLabelValue}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: serverNamespace}},
		},
		}},
		{Ports: []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 80}}}, To: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: serviceSelector},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: serverNamespace}},
		}}},
		{To: []v1.NetworkPolicyPeer{{IPBlock: &v1.IPBlock{CIDR: "8.8.8.8/32"}}}, Ports: make([]v1.NetworkPolicyPort, 0)}}

	egressNetpol := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameClient.Name,
			Namespace: networkPolicyNamespacedNameClient.Namespace,
			Labels:    map[string]string{otterizev2.OtterizeNetworkPolicy: formattedClient},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.OtterizeServiceLabelKey: formattedClient}},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			Egress:      egressRules,
		},
	}
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&egressNetpol))
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), egressNetpol.Namespace, gomock.Any())

	ingressRulesNotSVC := []v1.NetworkPolicyIngressRule{
		{From: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{fmt.Sprintf(otterizev2.OtterizeAccessLabelKey, formattedServer): "true"}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: testNamespace}},
		}}}}

	netpolNotSVC := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameServer.Name,
			Namespace: networkPolicyNamespacedNameServer.Namespace,
			Labels:    map[string]string{otterizev2.OtterizeNetworkPolicy: formattedServer},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.OtterizeServiceLabelKey: formattedServer}},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			Ingress:     ingressRulesNotSVC,
		},
	}

	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&netpolNotSVC))
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), netpolNotSVC.Namespace, gomock.Any())

	ingressRulesToSVC := []v1.NetworkPolicyIngressRule{
		{Ports: []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 80}}}, From: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{fmt.Sprintf(otterizev2.OtterizeSvcAccessLabelKey, formattedServerService): "true"}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: testNamespace}},
		}}}}

	netpolToSVC := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameServerService.Name,
			Namespace: networkPolicyNamespacedNameServerService.Namespace,
			Labels:    map[string]string{otterizev2.OtterizeNetworkPolicy: formattedServerService},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			Ingress:     ingressRulesToSVC,
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

func (s *AllBuildersTestSuite) TestCreateEveryRuleKindShouldCreateSeparatePolicies() {
	s.Reconciler.CreateSeparateEgressIngressPolicies = true
	serviceName := "test-client"
	serverName := "test-server"
	serverNamespace := testServerNamespace
	formattedServer := (&serviceidentity.ServiceIdentity{Name: serverName, Namespace: serverNamespace}).GetFormattedOtterizeIdentityWithoutKind()
	formattedServerService := (&serviceidentity.ServiceIdentity{Name: serverName, Namespace: serverNamespace, Kind: serviceidentity.KindService}).GetFormattedOtterizeIdentityWithKind()
	formattedClient := (&serviceidentity.ServiceIdentity{Name: serviceName, Namespace: testNamespace}).GetFormattedOtterizeIdentityWithoutKind()
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      "client-intents",
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}
	intentsSpec := &otterizev2.IntentsSpec{
		Workload: otterizev2.Workload{Name: serviceName},
		Targets: []otterizev2.Target{
			{
				Kubernetes: &otterizev2.KubernetesTarget{Name: fmt.Sprintf("%s.%s", serverName, serverNamespace)},
			},
			{
				Kubernetes: &otterizev2.KubernetesTarget{Name: fmt.Sprintf("%s.%s", serverName, serverNamespace), Kind: serviceidentity.KindService},
			},
			{
				Internet: &otterizev2.Internet{Ips: []string{"8.8.8.8/32"}},
			},
		},
	}

	s.expectGetAllEffectivePolicies([]otterizev2.ClientIntents{{Spec: intentsSpec, ObjectMeta: metav1.ObjectMeta{Name: namespacedName.Name, Namespace: namespacedName.Namespace}}})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedNameClient := types.NamespacedName{
		Namespace: testNamespace,
		Name:      "test-client-egress",
	}
	networkPolicyNamespacedNameServer := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      "test-server-ingress",
	}
	networkPolicyNamespacedNameServerService := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      "test-server-service-ingress",
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
	svc := s.addExpectedKubernetesServiceCall("test-server", serverNamespace, []corev1.ServicePort{{TargetPort: intstr.IntOrString{IntVal: 80}}}, serviceSelector)

	egressRules := []v1.NetworkPolicyEgressRule{
		{To: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.OtterizeServiceLabelKey: formattedServer}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: serverNamespace}},
		}}},
		{To: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{testServerServiceLabelKey: testServerServiceLabelValue}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: serverNamespace}},
		},
		}},
		{Ports: []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 80}}}, To: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: serviceSelector},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: serverNamespace}},
		}}},
		{To: []v1.NetworkPolicyPeer{{IPBlock: &v1.IPBlock{CIDR: "8.8.8.8/32"}}}, Ports: make([]v1.NetworkPolicyPort, 0)}}

	egressNetpol := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameClient.Name,
			Namespace: networkPolicyNamespacedNameClient.Namespace,
			Labels:    map[string]string{otterizev2.OtterizeNetworkPolicy: formattedClient},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.OtterizeServiceLabelKey: formattedClient}},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			Egress:      egressRules,
		},
	}
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&egressNetpol))
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), egressNetpol.Namespace, gomock.Any())

	ingressRulesNotSVC := []v1.NetworkPolicyIngressRule{
		{From: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{fmt.Sprintf(otterizev2.OtterizeAccessLabelKey, formattedServer): "true"}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: testNamespace}},
		}}}}

	netpolNotSVC := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameServer.Name,
			Namespace: networkPolicyNamespacedNameServer.Namespace,
			Labels:    map[string]string{otterizev2.OtterizeNetworkPolicy: formattedServer},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.OtterizeServiceLabelKey: formattedServer}},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			Ingress:     ingressRulesNotSVC,
		},
	}

	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&netpolNotSVC))
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), netpolNotSVC.Namespace, gomock.Any())

	ingressRulesToSVC := []v1.NetworkPolicyIngressRule{
		{Ports: []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 80}}}, From: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{fmt.Sprintf(otterizev2.OtterizeSvcAccessLabelKey, formattedServerService): "true"}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: testNamespace}},
		}}}}

	netpolToSVC := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameServerService.Name,
			Namespace: networkPolicyNamespacedNameServerService.Namespace,
			Labels:    map[string]string{otterizev2.OtterizeNetworkPolicy: formattedServerService},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			Ingress:     ingressRulesToSVC,
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

func (s *AllBuildersTestSuite) TestCreateEveryRuleKindWithKinds() {
	serviceName := "test-client"
	serverName := "test-server"
	serverNamespace := testServerNamespace
	formattedServerWithoutKind := (&serviceidentity.ServiceIdentity{Name: serverName, Namespace: serverNamespace, Kind: "Deployment"}).GetFormattedOtterizeIdentityWithoutKind()
	formattedServerWithKind := (&serviceidentity.ServiceIdentity{Name: serverName, Namespace: serverNamespace, Kind: "Deployment"}).GetFormattedOtterizeIdentityWithKind()
	formattedServerService := (&serviceidentity.ServiceIdentity{Name: serverName, Namespace: serverNamespace, Kind: serviceidentity.KindService}).GetFormattedOtterizeIdentityWithKind()
	formattedClientWithoutKind := (&serviceidentity.ServiceIdentity{Name: serviceName, Namespace: testNamespace, Kind: "Deployment"}).GetFormattedOtterizeIdentityWithoutKind()
	formattedClientWithKind := (&serviceidentity.ServiceIdentity{Name: serviceName, Namespace: testNamespace, Kind: "Deployment"}).GetFormattedOtterizeIdentityWithKind()
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      "client-intents",
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}
	intentsSpec := &otterizev2.IntentsSpec{
		Workload: otterizev2.Workload{Name: serviceName, Kind: "Deployment"},
		Targets: []otterizev2.Target{
			{
				Kubernetes: &otterizev2.KubernetesTarget{Name: fmt.Sprintf("%s.%s", serverName, serverNamespace), Kind: "Deployment"},
			},
			{
				Service: &otterizev2.ServiceTarget{Name: fmt.Sprintf("%s.%s", serverName, serverNamespace)},
			},
			{
				Internet: &otterizev2.Internet{Ips: []string{"8.8.8.8/32"}},
			},
		},
	}

	s.expectGetAllEffectivePolicies([]otterizev2.ClientIntents{{Spec: intentsSpec, ObjectMeta: metav1.ObjectMeta{Name: namespacedName.Name, Namespace: namespacedName.Namespace}}})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedNameClient := types.NamespacedName{
		Namespace: testNamespace,
		Name:      "test-client-deployment-access",
	}
	networkPolicyNamespacedNameServer := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      "test-server-deployment-access",
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
	svc := s.addExpectedKubernetesServiceCall("test-server", serverNamespace, []corev1.ServicePort{{TargetPort: intstr.IntOrString{IntVal: 80}}}, serviceSelector)

	egressRules := []v1.NetworkPolicyEgressRule{
		{To: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.OtterizeServiceLabelKey: formattedServerWithoutKind}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: serverNamespace}},
		}}},
		{To: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{testServerServiceLabelKey: testServerServiceLabelValue}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: serverNamespace}},
		},
		}},
		{Ports: []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 80}}}, To: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: serviceSelector},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: serverNamespace}},
		}}},
		{To: []v1.NetworkPolicyPeer{{IPBlock: &v1.IPBlock{CIDR: "8.8.8.8/32"}}}, Ports: make([]v1.NetworkPolicyPort, 0)}}

	egressNetpol := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameClient.Name,
			Namespace: networkPolicyNamespacedNameClient.Namespace,
			Labels:    map[string]string{otterizev2.OtterizeNetworkPolicy: formattedClientWithKind},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.OtterizeServiceLabelKey: formattedClientWithoutKind, otterizev2.OtterizeOwnerKindLabelKey: "Deployment"}},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			Egress:      egressRules,
		},
	}
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&egressNetpol))
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), egressNetpol.Namespace, gomock.Any())

	ingressRulesNotSVC := []v1.NetworkPolicyIngressRule{
		{From: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{fmt.Sprintf(otterizev2.OtterizeAccessLabelKey, formattedServerWithKind): "true"}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: testNamespace}},
		}}}}

	netpolNotSVC := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameServer.Name,
			Namespace: networkPolicyNamespacedNameServer.Namespace,
			Labels:    map[string]string{otterizev2.OtterizeNetworkPolicy: formattedServerWithKind},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev2.OtterizeServiceLabelKey:   formattedServerWithoutKind,
					otterizev2.OtterizeOwnerKindLabelKey: "Deployment",
				},
			},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			Ingress:     ingressRulesNotSVC,
		},
	}

	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(&netpolNotSVC))
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), netpolNotSVC.Namespace, gomock.Any())

	ingressRulesToSVC := []v1.NetworkPolicyIngressRule{
		{Ports: []v1.NetworkPolicyPort{{Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 80}}}, From: []v1.NetworkPolicyPeer{{
			PodSelector:       &metav1.LabelSelector{MatchLabels: map[string]string{fmt.Sprintf(otterizev2.OtterizeSvcAccessLabelKey, formattedServerService): "true"}},
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{otterizev2.KubernetesStandardNamespaceNameLabelKey: testNamespace}},
		}}}}

	netpolToSVC := v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkPolicyNamespacedNameServerService.Name,
			Namespace: networkPolicyNamespacedNameServerService.Namespace,
			Labels:    map[string]string{otterizev2.OtterizeNetworkPolicy: formattedServerService},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: serviceSelector},
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			Ingress:     ingressRulesToSVC,
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
