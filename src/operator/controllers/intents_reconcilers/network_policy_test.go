package intents_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/testbase"
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

type NetworkPolicyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	Reconciler            *NetworkPolicyReconciler
	externalNetpolHandler *mocks.MockexternalNetpolHandler
}

func (s *NetworkPolicyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.externalNetpolHandler = mocks.NewMockexternalNetpolHandler(s.Controller)
	restrictToNamespaces := make([]string, 0)

	s.Reconciler = NewNetworkPolicyReconciler(
		s.Client,
		&runtime.Scheme{},
		s.externalNetpolHandler,
		restrictToNamespaces,
		true,
		true,
		false,
	)

	s.Reconciler.Recorder = s.Recorder
}

func (s *NetworkPolicyReconcilerTestSuite) TearDownTest() {
	viper.Reset()
	s.Reconciler = nil
	s.externalNetpolHandler = nil
	s.MocksSuiteBase.TearDownTest()
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
		false,
		nil,
	)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyWithProtectedServices() {
	viper.Set(operatorconfig.EnableProtectedServicesKey, true)
	clientIntentsName := "client-intents"
	policyName := "access-to-test-server-from-test-namespace"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	protectedServices := []otterizev1alpha2.ProtectedServices{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-protected-services",
			Namespace: testNamespace,
		},
		Spec: otterizev1alpha2.ProtectedServicesSpec{
			ProtectedServices: []otterizev1alpha2.ProtectedService{{Name: "test-server"}},
		},
	}}

	s.testCreateNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		true,
		protectedServices,
	)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicyWithProtectedServicesMultipleResources() {
	viper.Set(operatorconfig.EnableProtectedServicesKey, true)
	clientIntentsName := "client-intents"
	policyName := "access-to-test-server-from-test-namespace"
	serviceName := "test-client"
	serverNamespace := testNamespace
	formattedTargetServer := "test-server-test-namespace-8ddecb"

	protectedServices := []otterizev1alpha2.ProtectedServices{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-protected-services",
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServicesSpec{
				ProtectedServices: []otterizev1alpha2.ProtectedService{{Name: "other-server"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-protected-services",
				Namespace: testNamespace,
			},
			Spec: otterizev1alpha2.ProtectedServicesSpec{
				ProtectedServices: []otterizev1alpha2.ProtectedService{{Name: "test-server"}},
			},
		},
	}

	s.testCreateNetworkPolicy(
		clientIntentsName,
		serverNamespace,
		serviceName,
		policyName,
		formattedTargetServer,
		true,
		protectedServices,
	)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
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
		false,
		nil,
	)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
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
			controllerutil.AddFinalizer(intents, otterizev1alpha2.NetworkPolicyFinalizerName)
			intents.Spec = intentsSpec
			return nil
		})

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
	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(newPolicy), MatchPatch(client.MergeFrom(existingBadPolicy))).Return(nil)
	s.ignoreRemoveOrphan()

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
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
			Namespace: testNamespace,
		},
		Spec: intentsSpec,
	}

	// Initial call to get the ClientIntents object when reconciler starts
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			clientIntents.DeepCopyInto(intents)
			controllerutil.AddFinalizer(intents, otterizev1alpha2.NetworkPolicyFinalizerName)
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
			Key:      otterizev1alpha2.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny,
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

	serverLabelSelector := client.MatchingFields{otterizev1alpha2.OtterizeFormattedTargetServerIndexField: existingPolicy.Labels[otterizev1alpha2.OtterizeNetworkPolicy]}
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&otterizev1alpha2.ClientIntentsList{}),
		&serverLabelSelector).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ClientIntentsList, options ...client.ListOption) error {
			list.Items = []otterizev1alpha2.ClientIntents{clientIntents}
			return nil
		})

	orphanServerLabelSelector := client.MatchingFields{otterizev1alpha2.OtterizeFormattedTargetServerIndexField: orphanPolicy.Labels[otterizev1alpha2.OtterizeNetworkPolicy]}
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
	s.Equal(ctrl.Result{}, res)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
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
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			clientIntentsObj.DeepCopyInto(intents)
			controllerutil.AddFinalizer(intents, otterizev1alpha2.NetworkPolicyFinalizerName)
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
		&client.ListOptions{Namespace: testNamespace},
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
		formattedTargetServer,
		testNamespace,
	)

	emptyNetworkPolicy := &v1.NetworkPolicy{}
	s.Client.EXPECT().Get(gomock.Any(), networkPolicyNamespacedName, gomock.Eq(emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, networkPolicy *v1.NetworkPolicy, options ...client.ListOption) error {
			existingPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	s.Client.EXPECT().Delete(gomock.Any(), gomock.Eq(existingPolicy)).Return(nil)
	selector := labels.SelectorFromSet(labels.Set(map[string]string{
		otterizev1alpha2.OtterizeServerLabelKey: formattedTargetServer,
	}))
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), serverNamespace, selector)

	// Remove finalizer
	controllerutil.AddFinalizer(&clientIntentsObj, otterizev1alpha2.NetworkPolicyFinalizerName)
	controllerutil.RemoveFinalizer(&clientIntentsObj, otterizev1alpha2.NetworkPolicyFinalizerName)
	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(&clientIntentsObj)).Return(nil)
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func (s *NetworkPolicyReconcilerTestSuite) testCreateNetworkPolicy(
	clientIntentsName string,
	serverNamespace string,
	serviceName string,
	policyName string,
	formattedTargetServer string,
	protectedServicesEnabled bool,
	protectedServices []otterizev1alpha2.ProtectedServices,
) {
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
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
			controllerutil.AddFinalizer(intents, otterizev1alpha2.NetworkPolicyFinalizerName)
			intents.Spec = intentsSpec
			return nil
		})

	if protectedServicesEnabled {
		s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServicesList{}), client.InNamespace(serverNamespace)).DoAndReturn(
			func(ctx context.Context, list *otterizev1alpha2.ProtectedServicesList, opts ...client.ListOption) error {
				list.Items = append(list.Items, protectedServices...)
				return nil
			})
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
		otterizev1alpha2.OtterizeServerLabelKey: formattedTargetServer,
	}))

	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), serverNamespace, selector)
	s.ignoreRemoveOrphan()

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func (s *NetworkPolicyReconcilerTestSuite) ignoreRemoveOrphan() {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev1alpha2.OtterizeNetworkPolicyServiceDefaultDeny,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
	}})
	s.Require().NoError(err)

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector}).Return(nil)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyFinalizerAdded() {
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
			Namespace: testNamespace,
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
	controllerutil.AddFinalizer(&intentsWithFinalizer, otterizev1alpha2.NetworkPolicyFinalizerName)
	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(&intentsWithFinalizer)).Return(nil)

	// Just assume policy exist because the rest of the flow is tested in other tests
	existingPolicy := networkPolicyTemplate(
		policyName,
		serverNamespace,
		formattedTargetServer,
		testNamespace,
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
	s.Equal(ctrl.Result{}, res)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
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
				otterizev1alpha2.OtterizeNetworkPolicy: formattedTargetServer,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedTargetServer,
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				{
					From: []v1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									fmt.Sprintf(
										otterizev1alpha2.OtterizeAccessLabelKey, formattedTargetServer): "true",
								},
							},
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									otterizev1alpha2.OtterizeNamespaceLabelKey: intentsObjNamespace,
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
	viper.Set(operatorconfig.EnableProtectedServicesKey, true)
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	serverName := "test-server"

	protectedServices := otterizev1alpha2.ProtectedServices{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-protected-services",
			Namespace: serverNamespace,
		},
		Spec: otterizev1alpha2.ProtectedServicesSpec{
			ProtectedServices: []otterizev1alpha2.ProtectedService{{Name: "other-server"}},
		},
	}

	protectedServicesResources := []otterizev1alpha2.ProtectedServices{protectedServices}
	s.testServerNotProtected(clientIntentsName, serverName, serverNamespace, serviceName, protectedServicesResources)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestServerNotInProtectionList_EmptyList() {
	viper.Set(operatorconfig.EnableProtectedServicesKey, true)
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	serverName := "test-server"

	protectedServices := otterizev1alpha2.ProtectedServices{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-protected-services",
			Namespace: serverNamespace,
		},
		Spec: otterizev1alpha2.ProtectedServicesSpec{
			ProtectedServices: []otterizev1alpha2.ProtectedService{},
		},
	}

	protectedServicesResources := []otterizev1alpha2.ProtectedServices{protectedServices}
	s.testServerNotProtected(clientIntentsName, serverName, serverNamespace, serviceName, protectedServicesResources)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestServerNotInProtectionList_NoResource() {
	viper.Set(operatorconfig.EnableProtectedServicesKey, true)
	clientIntentsName := "client-intents"
	serviceName := "test-client"
	serverNamespace := "other-namespace"
	serverName := "test-server"

	s.testServerNotProtected(clientIntentsName, serverName, serverNamespace, serviceName, []otterizev1alpha2.ProtectedServices{})
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) testServerNotProtected(clientIntentsName string, serverName string, serverNamespace string, serviceName string, protectedServices []otterizev1alpha2.ProtectedServices) {
	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
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
	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			controllerutil.AddFinalizer(intents, otterizev1alpha2.NetworkPolicyFinalizerName)
			intents.Spec = intentsSpec
			return nil
		})

	s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev1alpha2.ProtectedServicesList{}), client.InNamespace(serverNamespace)).DoAndReturn(
		func(ctx context.Context, list *otterizev1alpha2.ProtectedServicesList, opts ...client.ListOption) error {
			list.Items = append(list.Items, protectedServices...)
			return nil
		})

	s.ignoreRemoveOrphan()

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkPolicyCreateEnforcementDisabled() {
	s.Reconciler.enableNetworkPolicyCreation = false

	s.testEnforcementDisabled()
	s.ExpectEvent(ReasonNetworkPolicyCreationDisabled)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNetworkGlobalEnforcementDisabled() {
	s.Reconciler.enforcementEnabledGlobally = false

	s.testEnforcementDisabled()
	s.ExpectEvent(ReasonEnforcementGloballyDisabled)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNotInWatchedNamespaces() {
	s.Reconciler.RestrictToNamespaces = []string{"namespace-you-never-heard-of"}

	s.testEnforcementDisabled()
	s.ExpectEvent(ReasonNamespaceNotAllowed)
	s.ExpectEvent(ReasonCreatedNetworkPolicies)
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
			controllerutil.AddFinalizer(intents, otterizev1alpha2.NetworkPolicyFinalizerName)
			intents.Spec = intentsSpec
			return nil
		})

	s.ignoreRemoveOrphan()

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func (s *NetworkPolicyReconcilerTestSuite) TestPolicyNotDeletedForTwoClientsWithSameServer() {
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
			Namespace:         testNamespace,
			DeletionTimestamp: &metav1.Time{Time: time.Date(2020, 12, 1, 17, 14, 0, 0, time.UTC)},
		},
		Spec: intentsSpec,
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			clientIntentsObj.DeepCopyInto(intents)
			controllerutil.AddFinalizer(intents, otterizev1alpha2.NetworkPolicyFinalizerName)
			return nil
		})

	// Search for client intents with the same target server and delete them if it's the last client intent pointing to the server
	emptyIntentsList := &otterizev1alpha2.ClientIntentsList{}

	otherClientIntentsInTheNamespace := otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "other-client-intents",
			Namespace: testNamespace,
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
		&client.ListOptions{Namespace: testNamespace},
	).DoAndReturn(func(ctx context.Context, list *otterizev1alpha2.ClientIntentsList, opts ...client.ListOption) error {
		intentsList.DeepCopyInto(list)
		return nil
	})

	// Shouldn't remove NetworkPolicy from this namespace
	// because there is another client with the same target server

	// Remove finalizer from the ClientIntents object
	controllerutil.AddFinalizer(&clientIntentsObj, otterizev1alpha2.NetworkPolicyFinalizerName)
	controllerutil.RemoveFinalizer(&clientIntentsObj, otterizev1alpha2.NetworkPolicyFinalizerName)
	s.Client.EXPECT().Update(gomock.Any(), gomock.Eq(&clientIntentsObj)).Return(nil)
	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func TestNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyReconcilerTestSuite))
}
