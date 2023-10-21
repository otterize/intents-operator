package egress_network_policy

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"testing"
	"time"
)

const (
	protectedServicesResourceName        = "staging-protected-services"
	protectedService                     = "test-service"
	protectedServiceFormattedName        = "test-service-test-namespace-b0207e"
	anotherProtectedServiceResourceName  = "protect-other-services"
	anotherProtectedService              = "other-test-service"
	anotherProtectedServiceFormattedName = "other-test-service-test-namespace-398a04"
	testServerNamespace                  = "test-server-namespace"
	testClientNamespace                  = "test-client-namespace"
)

type EgressNetworkPolicyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler            *NetworkPolicyReconciler
	externalNetpolHandler *mocks.MockexternalNetpolHandler
}

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.externalNetpolHandler = mocks.NewMockexternalNetpolHandler(s.Controller)
	restrictToNamespaces := make([]string, 0)

	s.Reconciler = NewNetworkPolicyReconciler(
		s.Client,
		&runtime.Scheme{},
		restrictToNamespaces,
		true,
		true,
	)

	s.Reconciler.Recorder = s.Recorder
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.Reconciler = nil
	s.externalNetpolHandler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "egress-to-test-server-from-test-client-namespace"
	serviceName := "test-client"
	serverNamespace := testServerNamespace
	clientNamespace := testClientNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	s.testCreateNetworkPolicy(
		clientIntentsName,
		clientNamespace,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		true,
		nil,
	)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyWithProtectedServices() {
	clientIntentsName := "client-intents"
	policyName := "egress-to-test-server-from-test-client-namespace"
	serviceName := "test-client"
	serverNamespace := testServerNamespace
	clientNamespace := testClientNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	protectedService := []otterizev1alpha2.ProtectedService{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-protected-services",
			Namespace: testServerNamespace,
		},
		Spec: otterizev1alpha2.ProtectedServiceSpec{
			Name: "test-server",
		},
	}}

	s.testCreateNetworkPolicy(
		clientIntentsName,
		clientNamespace,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		false,
		protectedService,
	)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyWithProtectedServicesMultipleResources() {
	clientIntentsName := "client-intents"
	policyName := "egress-to-test-server-from-test-client-namespace"
	serviceName := "test-client"
	serverNamespace := testServerNamespace
	clientNamespace := testClientNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	protectedServices := []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-protected-services",
				Namespace: testServerNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: "other-server",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-protected-services-2",
				Namespace: testServerNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: "test-server",
			},
		},
	}

	s.testCreateNetworkPolicy(
		clientIntentsName,
		clientNamespace,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		false,
		protectedServices,
	)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateCrossNamespace() {
	clientIntentsName := "client-intents"
	policyName := "egress-to-test-server-from-test-client-namespace"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	clientNamespace := testClientNamespace
	formattedTargetServer := "test-server-other-namespace-f6a461"

	s.testCreateNetworkPolicy(
		clientIntentsName,
		clientNamespace,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		true,
		nil,
	)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCleanup() {
	clientIntentsName := "client-intents"
	policyName := "egress-to-test-server-from-test-client-namespace"
	serviceName := "test-client"
	serverNamespace := testServerNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	s.testCleanNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
	)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCleanupCrossNamespace() {
	clientIntentsName := "client-intents"
	policyName := "egress-to-test-server-from-test-client-namespace"
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

func (s *EgressNetworkPolicyReconcilerTestSuite) TestUpdateNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "egress-to-test-server-from-test-client-namespace"
	serviceName := "test-client"
	serverNamespace := testServerNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	namespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			controllerutil.AddFinalizer(intents, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
			intents.Namespace = testClientNamespace
			intents.Spec = intentsSpec
			return nil
		})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      policyName,
	}

	newPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		"test-client-test-client-namespac-edb3a2",
		formattedTargetServer,
		testClientNamespace,
	)
	existingBadPolicy := newPolicy.DeepCopy()
	existingBadPolicy.Spec.Egress[0].To[0].PodSelector.MatchLabels["another-label"] = "just to create diff from real policy"
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingBadPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	// Update NetworkPolicy
	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(newPolicy), intents_reconcilers.MatchPatch(client.MergeFrom(existingBadPolicy))).Return(nil)
	s.ignoreRemoveOrphan()

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestRemoveOrphanNetworkPolicy() {
	clientIntentsName := "client-intents"
	policyName := "egress-to-test-server-from-test-server-namespace"
	serviceName := "test-client"
	serverNamespace := testServerNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	namespacedName := types.NamespacedName{
		Namespace: testServerNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}
	clientIntents := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testServerNamespace,
		},
		Spec: intentsSpec,
	}

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			controllerutil.AddFinalizer(intents, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
			return nil
		})

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}

	existingPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		"test-client-test-server-namespac-8e2cac",
		formattedTargetServer,
		testServerNamespace,
	)
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha2.OtterizeEgressNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
	}})
	s.Require().NoError(err)

	nonExistingServer := "old-non-existing-server"
	orphanPolicy := networkPolicyTemplate(
		"egress-to-old-non-existing-server-from-test-server-namespace",
		serverNamespace,
		"test-client-test-server-namespac-8e2cac",
		nonExistingServer,
		testServerNamespace,
	)
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector}).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, options ...client.ListOption) error {
			list.Items = []v1.NetworkPolicy{*existingPolicy, *orphanPolicy}
			return nil
		})

	serverLabelSelector := client.MatchingFields{otterizev1alpha2.OtterizeFormattedTargetServerIndexField: existingPolicy.Labels[otterizev1alpha2.OtterizeEgressNetworkPolicy]}
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&otterizev1alpha2.ClientIntentsList{}),
		&serverLabelSelector).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ClientIntentsList, options ...client.ListOption) error {
			list.Items = []otterizev1alpha2.ClientIntents{clientIntents}
			return nil
		})

	orphanServerLabelSelector := client.MatchingFields{otterizev1alpha2.OtterizeFormattedTargetServerIndexField: orphanPolicy.Labels[otterizev1alpha2.OtterizeEgressNetworkPolicy]}
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&otterizev1alpha2.ClientIntentsList{}),
		&orphanServerLabelSelector).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ClientIntentsList, options ...client.ListOption) error {
			// There are no ClientIntents for this NetworkPolicy, so it should be deleted
			list.Items = []otterizev1alpha2.ClientIntents{}
			return nil
		})

	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), orphanPolicy)
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(orphanPolicy)).Return(nil)
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) testCleanNetworkPolicy(clientIntentsName string, serverNamespace string, serviceName string, policyName string, formattedTargetServer string) {
	namespacedName := types.NamespacedName{
		Namespace: testServerNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	clientIntentsObj := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         testServerNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			clientIntentsObj.DeepCopyInto(intents)
			controllerutil.AddFinalizer(intents, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
			return nil
		})

	// Search for client intents with the same target server and delete them if it's the last client intent pointing to the server
	emptyIntentsList := &otterizev1alpha2.ClientIntentsList{}
	intentsList := &otterizev1alpha2.ClientIntentsList{
		Items: []otterizev1alpha2.ClientIntents{
			clientIntentsObj,
		},
	}
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(emptyIntentsList),
		&client.MatchingFields{otterizev1alpha2.OtterizeTargetServerIndexField: serverName},
		&client.ListOptions{Namespace: testServerNamespace},
	).DoAndReturn(func(ctx context.Context, list *otterizev1alpha2.ClientIntentsList, opts ...client.ListOption) error {
		intentsList.DeepCopyInto(list)
		return nil
	})

	// Remove network policy:
	// 1. get the network policy
	// 2. delete it
	// 3.call external netpol handler

	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}

	existingPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		"test-client-test-client-namespac-edb3a2",
		formattedTargetServer,
		testServerNamespace,
	)

	emptyNetworkPolicy := &v1.NetworkPolicy{}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), existingPolicy)
	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(existingPolicy)).Return(nil)
	selector := labels.SelectorFromSet(labels.Set(map[string]string{
		otterizev1alpha2.OtterizeServerLabelKey: formattedTargetServer,
	}))
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), serverNamespace, selector)

	// Remove finalizer
	controllerutil.AddFinalizer(&clientIntentsObj, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
	controllerutil.RemoveFinalizer(&clientIntentsObj, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(&clientIntentsObj)).Return(nil)
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) testCreateNetworkPolicy(
	clientIntentsName string,
	clientNamespace string,
	serverNamespace string,
	serviceName string,
	policyName string,
	formattedTargetServer string,
	defaultEnforcementState bool,
	protectedServices []otterizev1alpha2.ProtectedService,
) {
	s.Reconciler.enforcementDefaultState = defaultEnforcementState
	namespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			controllerutil.AddFinalizer(intents, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
			intents.Namespace = clientNamespace
			intents.Spec = intentsSpec
			return nil
		})

	if defaultEnforcementState == false {
		s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), gomock.Any()).DoAndReturn(
			func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
				list.Items = append(list.Items, protectedServices...)
				return nil
			})
	}

	// Search for existing NetworkPolicy
	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: clientNamespace,
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
		"test-client-test-client-namespac-edb3a2",
		formattedTargetServer,
		testClientNamespace,
	)
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	s.ignoreRemoveOrphan()

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) ignoreRemoveOrphan() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha2.OtterizeEgressNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
	}})
	s.Require().NoError(err)

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector}).Return(nil)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkPolicyFinalizerAdded() {
	clientIntentsName := "client-intents"
	policyName := "egress-to-test-server-from-test-client-namespace"
	serviceName := "test-client"
	serverNamespace := testServerNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	namespacedName := types.NamespacedName{
		Namespace: testServerNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	intentsWithoutFinalizer := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientIntentsName,
			Namespace: testServerNamespace,
		},
		Spec: intentsSpec,
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			intentsWithoutFinalizer.DeepCopyInto(intents)
			return nil
		})

	intentsWithFinalizer := otterizev1alpha2.ClientIntents{}
	intentsWithoutFinalizer.DeepCopyInto(&intentsWithFinalizer)
	// Add finalizer to ClientIntents
	controllerutil.AddFinalizer(&intentsWithFinalizer, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(&intentsWithFinalizer)).Return(nil)

	// Just assume policy exist because the rest of the flow is tested in other tests
	existingPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		"test-client-test-client-namespac-edb3a2",
		formattedTargetServer,
		testServerNamespace,
	)

	emptyNetworkPolicy := &v1.NetworkPolicy{}
	networkPolicyNamespacedName := types.NamespacedName{
		Namespace: serverNamespace,
		Name:      policyName,
	}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	s.ignoreRemoveOrphan()

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
	s.ExpectEvent(consts.ReasonCreatedEgressNetworkPolicies)
}

func networkPolicyTemplate(
	policyName string,
	targetNamespace string,
	formattedTargetClient string,
	formattedTargetServer string,
	intentsObjNamespace string,
) *v1.NetworkPolicy {
	return &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: intentsObjNamespace,
			Labels: map[string]string{
				otterizev1alpha2.OtterizeEgressNetworkPolicy: formattedTargetClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeClientLabelKey: formattedTargetClient,
				},
			},
			Egress: []v1.NetworkPolicyEgressRule{
				{
					To: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha2.OtterizeServerLabelKey: formattedTargetServer,
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha2.OtterizeNamespaceLabelKey: targetNamespace,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestServerNotInProtectionList() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	serverName := "test-server"
	s.Reconciler.enforcementDefaultState = false

	s.testServerNotProtected(clientIntentsName, serverName, serverNamespace, serviceName)
	s.ExpectEvent(consts.ReasonEnforcementDefaultOff)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) testServerNotProtected(clientIntentsName string, serverName string, serverNamespace string, serviceName string) {
	namespacedName := types.NamespacedName{
		Namespace: testServerNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverFullName := fmt.Sprintf("%s.%s", serverName, serverNamespace)
	intentsSpec := &otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverFullName,
			},
		},
	}

	// Initial call to get the ClientIntents object when reconciler starts
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&otterizev1alpha2.ClientIntents{})).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			controllerutil.AddFinalizer(intents, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
			intents.Namespace = testClientNamespace
			intents.Spec = intentsSpec
			return nil
		})

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), gomock.Any()).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
			return nil
		})

	s.ignoreRemoveOrphan()

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateEnforcementDisabled() {
	s.Reconciler.enableNetworkPolicyCreation = false

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNetworkPolicyCreationDisabled)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNetworkGlobalEnforcementDisabled() {
	s.Reconciler.enforcementDefaultState = false

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonEnforcementDefaultOff)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNotInWatchedNamespaces() {
	s.Reconciler.RestrictToNamespaces = []string{"namespace-you-never-heard-of"}

	s.testEnforcementDisabled()
	s.ExpectEvent(consts.ReasonNamespaceNotAllowed)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) testEnforcementDisabled() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "other-namespace"

	namespacedName := types.NamespacedName{
		Namespace: testServerNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			controllerutil.AddFinalizer(intents, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
			intents.Namespace = testClientNamespace
			intents.Spec = intentsSpec
			return nil
		})

	s.ignoreRemoveOrphan()
	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), gomock.Any()).AnyTimes().DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
			return nil
		})

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestPolicyNotDeletedForTwoClientsWithSameServer() {
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "other-namespace"

	namespacedName := types.NamespacedName{
		Namespace: testServerNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	clientIntentsObj := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:              clientIntentsName,
			Namespace:         testServerNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			clientIntentsObj.DeepCopyInto(intents)
			controllerutil.AddFinalizer(intents, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
			return nil
		})

	// Search for client intents with the same target server and delete them if it's the last client intent pointing to the server
	emptyIntentsList := &otterizev1alpha2.ClientIntentsList{}

	otherClientIntentsInTheNamespace := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-client-intents",
			Namespace: testServerNamespace,
		},
		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{Name: "other-client"},
			Calls: []otterizev1alpha2.Intent{
				{
					Name: serverName,
				},
			},
		},
	}
	intentsList := &otterizev1alpha2.ClientIntentsList{
		Items: []otterizev1alpha2.ClientIntents{
			clientIntentsObj,
			otherClientIntentsInTheNamespace,
		},
	}
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(emptyIntentsList),
		&client.MatchingFields{otterizev1alpha2.OtterizeTargetServerIndexField: serverName},
		&client.ListOptions{Namespace: testServerNamespace},
	).DoAndReturn(func(ctx context.Context, list *otterizev1alpha2.ClientIntentsList, opts ...client.ListOption) error {
		intentsList.DeepCopyInto(list)
		return nil
	})

	// Shouldn't remove NetworkPolicy from this namespace
	// because there is another client with the same target server

	// Remove finalizer from the ClientIntents object
	controllerutil.AddFinalizer(&clientIntentsObj, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
	controllerutil.RemoveFinalizer(&clientIntentsObj, otterizev1alpha2.EgressNetworkPolicyFinalizerName)
	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(&clientIntentsObj)).Return(nil)
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestAllServerAreProtected() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha2.OtterizeEgressNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
	}})
	s.Require().NoError(err)

	networkPolicies := []v1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-name",
				Namespace: testServerNamespace,
				Labels: map[string]string{
					otterizev1alpha2.OtterizeEgressNetworkPolicy: protectedServiceFormattedName,
				},
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha2.OtterizeServerLabelKey: protectedServiceFormattedName,
					},
				},
				Ingress: []v1.NetworkPolicyIngressRule{
					{
						From: []v1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										fmt.Sprintf(
											otterizev1alpha2.OtterizeAccessLabelKey, protectedServiceFormattedName): "true",
									},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										otterizev1alpha2.OtterizeNamespaceLabelKey: testServerNamespace,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "another-policy-name",
				Namespace: testServerNamespace,
				Labels: map[string]string{
					otterizev1alpha2.OtterizeEgressNetworkPolicy: anotherProtectedServiceFormattedName,
				},
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha2.OtterizeServerLabelKey: anotherProtectedServiceFormattedName,
					},
				},
				Ingress: []v1.NetworkPolicyIngressRule{
					{
						From: []v1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										fmt.Sprintf(
											otterizev1alpha2.OtterizeAccessLabelKey, anotherProtectedServiceFormattedName): "true",
									},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										otterizev1alpha2.OtterizeNamespaceLabelKey: testServerNamespace,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&v1.NetworkPolicyList{}),
		&client.ListOptions{Namespace: testServerNamespace, LabelSelector: selector},
	).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, networkPolicies...)
			return nil
		})

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testServerNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: protectedService,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      anotherProtectedServiceResourceName,
				Namespace: testServerNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: anotherProtectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), &client.ListOptions{Namespace: testServerNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
			protectedServicesResources.DeepCopyInto(list)
			return nil
		})

	err = s.Reconciler.CleanPoliciesFromUnprotectedServices(context.Background(), testServerNamespace)
	s.Require().NoError(err)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestUnprotectedServerWithAccessPolicy() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha2.OtterizeEgressNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
	}})
	s.Require().NoError(err)

	networkPolicies := []v1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-name",
				Namespace: testServerNamespace,
				Labels: map[string]string{
					otterizev1alpha2.OtterizeEgressNetworkPolicy: protectedServiceFormattedName,
				},
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha2.OtterizeServerLabelKey: protectedServiceFormattedName,
					},
				},
				Ingress: []v1.NetworkPolicyIngressRule{
					{
						From: []v1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										fmt.Sprintf(
											otterizev1alpha2.OtterizeAccessLabelKey, protectedServiceFormattedName): "true",
									},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										otterizev1alpha2.OtterizeNamespaceLabelKey: testServerNamespace,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "another-policy-name",
				Namespace: testServerNamespace,
				Labels: map[string]string{
					otterizev1alpha2.OtterizeEgressNetworkPolicy: anotherProtectedServiceFormattedName,
				},
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha2.OtterizeServerLabelKey: anotherProtectedServiceFormattedName,
					},
				},
				Ingress: []v1.NetworkPolicyIngressRule{
					{
						From: []v1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										fmt.Sprintf(
											otterizev1alpha2.OtterizeAccessLabelKey, anotherProtectedServiceFormattedName): "true",
									},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										otterizev1alpha2.OtterizeNamespaceLabelKey: testServerNamespace,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&v1.NetworkPolicyList{}),
		&client.ListOptions{Namespace: testServerNamespace, LabelSelector: selector},
	).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, networkPolicies...)
			return nil
		})

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testServerNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: protectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), &client.ListOptions{Namespace: testServerNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
			protectedServicesResources.DeepCopyInto(list)
			return nil
		})

	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), &networkPolicies[1]).Times(1)
	s.Client.EXPECT().Delete(gomock.Any(), &networkPolicies[1]).Times(1)
	err = s.Reconciler.CleanPoliciesFromUnprotectedServices(context.Background(), testServerNamespace)
	s.Require().NoError(err)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestProtectedServiceInDeletionWithAccessPolicy() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha2.OtterizeEgressNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
	}})
	s.Require().NoError(err)

	networkPolicies := []v1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-name",
				Namespace: testServerNamespace,
				Labels: map[string]string{
					otterizev1alpha2.OtterizeEgressNetworkPolicy: protectedServiceFormattedName,
				},
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha2.OtterizeServerLabelKey: protectedServiceFormattedName,
					},
				},
				Ingress: []v1.NetworkPolicyIngressRule{
					{
						From: []v1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										fmt.Sprintf(
											otterizev1alpha2.OtterizeAccessLabelKey, protectedServiceFormattedName): "true",
									},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										otterizev1alpha2.OtterizeNamespaceLabelKey: testServerNamespace,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "another-policy-name",
				Namespace: testServerNamespace,
				Labels: map[string]string{
					otterizev1alpha2.OtterizeEgressNetworkPolicy: anotherProtectedServiceFormattedName,
				},
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha2.OtterizeServerLabelKey: anotherProtectedServiceFormattedName,
					},
				},
				Ingress: []v1.NetworkPolicyIngressRule{
					{
						From: []v1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										fmt.Sprintf(
											otterizev1alpha2.OtterizeAccessLabelKey, anotherProtectedServiceFormattedName): "true",
									},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										otterizev1alpha2.OtterizeNamespaceLabelKey: testServerNamespace,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&v1.NetworkPolicyList{}),
		&client.ListOptions{Namespace: testServerNamespace, LabelSelector: selector},
	).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, networkPolicies...)
			return nil
		})

	// One service is protected, the other has protectedService with deletion timestamp
	deletionTimestamp := metav1.Now()
	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testServerNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: protectedService,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              anotherProtectedServiceResourceName,
				Namespace:         testServerNamespace,
				DeletionTimestamp: &deletionTimestamp,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: anotherProtectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), &client.ListOptions{Namespace: testServerNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
			protectedServicesResources.DeepCopyInto(list)
			return nil
		})

	s.externalNetpolHandler.EXPECT().HandleBeforeAccessPolicyRemoval(gomock.Any(), &networkPolicies[1]).Times(1)
	s.Client.EXPECT().Delete(gomock.Any(), &networkPolicies[1]).Times(1)
	err = s.Reconciler.CleanPoliciesFromUnprotectedServices(context.Background(), testServerNamespace)
	s.Require().NoError(err)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestServerWithoutPolicyNothingShouldHappen() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha2.OtterizeEgressNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
	}})
	s.Require().NoError(err)

	networkPolicies := []v1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy-name",
				Namespace: testServerNamespace,
				Labels: map[string]string{
					otterizev1alpha2.OtterizeEgressNetworkPolicy: protectedServiceFormattedName,
				},
			},
			Spec: v1.NetworkPolicySpec{
				PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						otterizev1alpha2.OtterizeServerLabelKey: protectedServiceFormattedName,
					},
				},
				Ingress: []v1.NetworkPolicyIngressRule{
					{
						From: []v1.NetworkPolicyPeer{
							{
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										fmt.Sprintf(
											otterizev1alpha2.OtterizeAccessLabelKey, protectedServiceFormattedName): "true",
									},
								},
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										otterizev1alpha2.OtterizeNamespaceLabelKey: testServerNamespace,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&v1.NetworkPolicyList{}),
		&client.ListOptions{Namespace: testServerNamespace, LabelSelector: selector},
	).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			list.Items = append(list.Items, networkPolicies...)
			return nil
		})

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testServerNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: protectedService,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      anotherProtectedServiceResourceName,
				Namespace: testServerNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: anotherProtectedService,
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServiceList{}), &client.ListOptions{Namespace: testServerNamespace}).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServiceList, opts ...client.ListOption) error {
			protectedServicesResources.DeepCopyInto(list)
			return nil
		})

	err = s.Reconciler.CleanPoliciesFromUnprotectedServices(context.Background(), testServerNamespace)
	s.Require().NoError(err)
}

func (s *EgressNetworkPolicyReconcilerTestSuite) TestNoNetworkPolicies() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha2.OtterizeEgressNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
	}})
	s.Require().NoError(err)
	// Get all existing network policies
	// No network policies exist
	var networkPolicies v1.NetworkPolicyList
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&networkPolicies),
		&client.ListOptions{Namespace: testServerNamespace, LabelSelector: selector},
	).Return(nil).Times(1)

	var protectedServicesResources otterizev1alpha2.ProtectedServiceList
	protectedServicesResources.Items = []otterizev1alpha2.ProtectedService{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      protectedServicesResourceName,
				Namespace: testServerNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: protectedService,
			},
		},
	}

	err = s.Reconciler.CleanPoliciesFromUnprotectedServices(context.Background(), testServerNamespace)
	s.Require().NoError(err)
}

func TestEgressNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(EgressNetworkPolicyReconcilerTestSuite))
}
