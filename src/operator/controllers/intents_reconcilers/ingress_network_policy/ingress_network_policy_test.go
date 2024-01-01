package ingress_network_policy

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
	"time"
)

const (
	testNamespace = "test-namespace"
)

type NetworkPolicyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler            *NetworkPolicyApplier
	externalNetpolHandler *mocks.MockexternalNetpolHandler
	EPIntentsReconciler   *intents_reconcilers.ServiceEffectivePolicyIntentsReconciler
}

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func (s *NetworkPolicyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.externalNetpolHandler = mocks.NewMockexternalNetpolHandler(s.Controller)
	restrictToNamespaces := make([]string, 0)

	scheme := &runtime.Scheme{}
	s.Reconciler = NewNetworkPolicyApplier(
		s.Client,
		scheme,
		s.externalNetpolHandler,
		restrictToNamespaces,
		true,
		true,
		allowexternaltraffic.IfBlockedByOtterize,
	)

	syncer := effectivepolicy.NewSyncer(s.Client,
		scheme, s.Reconciler)
	s.EPIntentsReconciler = intents_reconcilers.NewServiceEffectiveIntentsReconciler(s.Client,
		scheme, syncer)

	s.Reconciler.Recorder = s.Recorder
	syncer.InjectableRecorder.Recorder = s.Recorder
	s.EPIntentsReconciler.Recorder = s.Recorder
}

func (s *NetworkPolicyReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.Reconciler = nil
	s.externalNetpolHandler = nil
	s.EPIntentsReconciler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *NetworkPolicyReconcilerTestSuite) ignoreRemoveOrphan() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicyServiceDefaultDeny,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
	}})
	s.Require().NoError(err)

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector}).Return(nil)
}

func (s *NetworkPolicyReconcilerTestSuite) testCreateNetworkPolicy(
	clientIntentsName string,
	serverNamespace string,
	serviceName string,
	policyName string,
	formattedTargetServer string,
	defaultEnforcementState bool,
	protectedServices []otterizev1alpha3.ProtectedService,
) {
	s.Reconciler.enforcementDefaultState = defaultEnforcementState
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: fmt.Sprintf("test-server.%s", serverNamespace),
			},
		},
	}

	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{{Spec: intentsSpec, ObjectMeta: metav1.ObjectMeta{Name: namespacedName.Name, Namespace: namespacedName.Namespace}}})

	if defaultEnforcementState == false {
		s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha3.ProtectedServiceList{}), gomock.Any()).DoAndReturn(
			func(ctx context.Context, list *otterizev1alpha3.ProtectedServiceList, opts ...client.ListOption) error {
				list.Items = append(list.Items, protectedServices...)
				return nil
			}).AnyTimes()
	}

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
	newPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	selector := labels.SelectorFromSet(labels.Set(map[string]string{
		otterizev1alpha3.OtterizeServerLabelKey: formattedTargetServer,
	}))

	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), serverNamespace, selector)
	s.ignoreRemoveOrphan()

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "access-to-test-server-from-test-namespace"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	s.testCreateNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		true,
		nil,
	)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyWithProtectedServices() {
	clientIntentsName := "client-intents"
	policyName := "access-to-test-server-from-test-namespace"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	protectedService := []otterizev1alpha3.ProtectedService{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-protected-services",
			Namespace: testNamespace,
		},
		Spec: otterizev1alpha3.ProtectedServiceSpec{
			Name: "test-server",
		},
	}}

	s.testCreateNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		false,
		protectedService,
	)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyWithProtectedServicesMultipleResources() {
	clientIntentsName := "client-intents"
	policyName := "access-to-test-server-from-test-namespace"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	protectedServices := []otterizev1alpha3.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-protected-services",
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha3.ProtectedServiceSpec{
				Name: "other-server",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-protected-services-2",
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha3.ProtectedServiceSpec{
				Name: "test-server",
			},
		},
	}

	s.testCreateNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		false,
		protectedServices,
	)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateCrossNamespace() {
	clientIntentsName := "client-intents"
	policyName := "access-to-test-server-from-test-namespace"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	formattedTargetServer := "test-server-other-namespace-f6a461"

	s.testCreateNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		true,
		nil,
	)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCleanup() {
	clientIntentsName := "client-intents"
	policyName := "access-to-test-server-from-test-namespace"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	s.testCleanNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
	)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCleanupCrossNamespace() {
	clientIntentsName := "client-intents"
	policyName := "access-to-test-server-from-test-namespace"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	formattedTargetServer := "test-server-other-namespace-f6a461"

	s.testCleanNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
	)
}

func (s *NetworkPolicyReconcilerTestSuite) TestUpdateNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "access-to-test-server-from-test-namespace"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}

	newPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)
	existingBadPolicy := newPolicy.DeepCopy()
	existingBadPolicy.Spec.Ingress[0].From[0].PodSelector.MatchLabels["another-label"] = "just to create diff from real policy"
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingBadPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	// Update NetworkPolicy
	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(newPolicy), intents_reconcilers.MatchPatch(client.MergeFrom(existingBadPolicy))).Return(nil)
	s.ignoreRemoveOrphan()

	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{{ObjectMeta: metav1.ObjectMeta{Namespace: req.Namespace, Name: req.Name}, Spec: intentsSpec}})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestRemoveOrphanNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "access-to-test-server-from-test-namespace"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}

	existingPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicyServiceDefaultDeny,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
	}})
	s.Require().NoError(err)

	nonExistingServer := "old-non-existing-server"
	orphanPolicy := networkPolicyTemplate(
		"access-to-old-non-existing-server-from-test-namespace",
		serverNamespace,
		nonExistingServer,
		testNamespace,
	)
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector}).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, options ...client.ListOption) error {
			list.Items = []v1.NetworkPolicy{*existingPolicy, *orphanPolicy}
			return nil
		})

	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), orphanPolicy)
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(orphanPolicy)).Return(nil)

	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{{ObjectMeta: metav1.ObjectMeta{Namespace: req.Namespace, Name: req.Name}, Spec: intentsSpec}})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) testCleanNetworkPolicy(clientIntentsName string, serverNamespace string, serviceName string, policyName string, formattedTargetServer string) {
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}

	clientIntentsObj := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}

	// Remove network policy:
	// 1. Calls remove orphan
	// 2. delete it
	// 3.call external netpol handler

	existingPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)

	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), existingPolicy)
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(existingPolicy)).Return(nil)
	// TODO: Why did we need it?
	//selector := labels.SelectorFromSet(labels.Set(map[string]string{
	//	otterizev1alpha3.OtterizeServerLabelKey: formattedTargetServer,
	//}))
	//s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), serverNamespace, selector)

	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntentsObj})
	s.expectRemoveOrphanFindsPolicies([]v1.NetworkPolicy{*existingPolicy})
	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) expectGetAllEffectivePolicies(clientIntents []otterizev1alpha3.ClientIntents) {
	var intentsList otterizev1alpha3.ClientIntentsList

	s.Client.EXPECT().List(gomock.Any(), &intentsList).DoAndReturn(func(_ context.Context, intents *otterizev1alpha3.ClientIntentsList, _ ...any) error {
		intents.Items = append(intents.Items, clientIntents...)
		return nil
	})

	// create service to ClientIntents pointing to it
	services := make(map[string][]otterizev1alpha3.ClientIntents)
	for _, clientIntent := range clientIntents {
		for _, intentCall := range clientIntent.GetCallsList() {
			server := otterizev1alpha3.GetFormattedOtterizeIdentity(intentCall.GetTargetServerName(), intentCall.GetTargetServerNamespace(clientIntent.Namespace))
			services[server] = append(services[server], clientIntent)
		}
	}

	matchFieldsPtr := &client.MatchingFields{}
	s.Client.EXPECT().List(
		gomock.Any(),
		&otterizev1alpha3.ClientIntentsList{},
		gomock.AssignableToTypeOf(matchFieldsPtr),
	).DoAndReturn(func(_ context.Context, intents *otterizev1alpha3.ClientIntentsList, args ...any) error {
		matchFields := args[0].(*client.MatchingFields)
		intents.Items, _ = services[(*matchFields)[otterizev1alpha3.OtterizeFormattedTargetServerIndexField]]
		return nil
	}).AnyTimes()

}

func (s *NetworkPolicyReconcilerTestSuite) expectRemoveOrphanFindsPolicies(netpols []v1.NetworkPolicy) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev1alpha3.OtterizeNetworkPolicyServiceDefaultDeny,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
	}})
	s.Require().NoError(err)

	s.Client.EXPECT().List(
		gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector},
	).DoAndReturn(
		func(_ context.Context, netpolList *v1.NetworkPolicyList, _ ...any) error {
			netpolList.Items = append(netpolList.Items, netpols...)
			return nil
		},
	)
}

func networkPolicyTemplate(
	policyName string,
	targetNamespace string,
	formattedTargetServer string,
	intentsObjNamespace string,
) *v1.NetworkPolicy {
	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: targetNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeNetworkPolicy: formattedTargetServer,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha3.OtterizeServerLabelKey: formattedTargetServer,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				{
					From: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									fmt.Sprintf(
										otterizev1alpha3.OtterizeAccessLabelKey, formattedTargetServer): "true",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha3.KubernetesStandardNamespaceNameLabelKey: intentsObjNamespace,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (s *NetworkPolicyReconcilerTestSuite) TestServerNotInProtectionList() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	serverName := "test-server"
	s.Reconciler.enforcementDefaultState = false

	s.testServerNotProtected(clientIntentsName, serverName, serverNamespace, serviceName)
	s.ExpectEvent(consts.ReasonEnforcementDefaultOff)
}

func (s *NetworkPolicyReconcilerTestSuite) testServerNotProtected(clientIntentsName string, serverName string, serverNamespace string, serviceName string) {
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverFullName := fmt.Sprintf("%s.%s", serverName, serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverFullName,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha3.ProtectedServiceList{}), gomock.Any()).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha3.ProtectedServiceList, opts ...client.ListOption) error {
			return nil
		}).AnyTimes()

	s.ignoreRemoveOrphan()

	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{{ObjectMeta: metav1.ObjectMeta{Namespace: req.Namespace, Name: req.Name}, Spec: intentsSpec}})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateEnforcementDisabled() {
	s.Reconciler.enableNetworkPolicyCreation = false

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNetworkPolicyCreationDisabled)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkGlobalEnforcementDisabled() {
	s.Reconciler.enforcementDefaultState = false

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonEnforcementDefaultOff)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNotInWatchedNamespaces() {
	s.Reconciler.RestrictToNamespaces = []string{"namespace-you-never-heard-of"}

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNamespaceNotAllowed)
}

func (s *NetworkPolicyReconcilerTestSuite) testEnforcementDisabled() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "other-namespace"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}

	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{{Spec: intentsSpec, ObjectMeta: metav1.ObjectMeta{Name: namespacedName.Name, Namespace: namespacedName.Namespace}}})

	s.ignoreRemoveOrphan()
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha3.ProtectedServiceList{}), gomock.Any()).AnyTimes().DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha3.ProtectedServiceList, opts ...client.ListOption) error {
			return nil
		})

	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) TestPolicyNotDeletedForTwoClientsWithSameServer() {
	// This test checks that if a netpol created by 2 different intents and 1 of them is being deleted,
	// then the netpol won't be deleted.
	// The flow of the reconciler should be:
	// 	  1. get all effective policies
	//    2. get existing policy and not update because it is equal
	//    3. check if deletion is needed (the answer should be no)
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := testNamespace
	policyName := "access-to-test-server-from-test-namespace"
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: serviceName},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: serverName,
			},
		},
	}

	clientIntentsObj := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}

	// Search for client intents with the same target server and delete them if it's the last client intent pointing to the server

	otherClientIntentsInTheNamespace := otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-client-intents",
			Namespace: testNamespace,
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: "other-client"},
			Calls: []otterizev1alpha3.Intent{
				{
					Name: serverName,
				},
			},
		},
	}

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}

	newPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
	)

	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			newPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	s.expectGetAllEffectivePolicies([]otterizev1alpha3.ClientIntents{clientIntentsObj, otherClientIntentsInTheNamespace})
	s.expectRemoveOrphanFindsPolicies([]v1.NetworkPolicy{*newPolicy})

	// Shouldn't remove NetworkPolicy from this namespace
	// because there is another client with the same target server
	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func TestNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyReconcilerTestSuite))
}
