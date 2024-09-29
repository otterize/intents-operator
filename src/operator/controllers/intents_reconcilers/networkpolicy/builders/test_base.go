package builders

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

const (
	testNamespace               = "test-namespace"
	testServerNamespace         = "test-server-namespace"
	testClientNamespace         = "test-client-namespace"
	testServerServiceLabelKey   = "app"
	testServerServiceLabelValue = "server"
)

type RulesBuilderTestSuiteBase struct {
	testbase.MocksSuiteBase
	Reconciler            *networkpolicy.Reconciler
	EPIntentsReconciler   *intents_reconcilers.ServiceEffectivePolicyIntentsReconciler
	externalNetpolHandler *mocks.MockExternalNetpolHandler
	scheme                *runtime.Scheme
}

func (s *RulesBuilderTestSuiteBase) SetupTest() {
	logrus.SetLevel(logrus.DebugLevel)
	s.MocksSuiteBase.SetupTest()
	s.externalNetpolHandler = mocks.NewMockExternalNetpolHandler(s.Controller)
	restrictToNamespaces := make([]string, 0)

	s.scheme = runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s.scheme))

	s.Reconciler = networkpolicy.NewReconciler(s.Client,
		s.scheme,
		s.externalNetpolHandler,
		restrictToNamespaces,
		nil,
		true,
		true,
		nil,
		nil)
	s.Reconciler.Recorder = s.Recorder

	serviceIdResolver := serviceidresolver.NewResolver(s.Client)
	epReconciler := effectivepolicy.NewGroupReconciler(s.Client,
		s.scheme, serviceIdResolver, s.Reconciler)
	s.EPIntentsReconciler = intents_reconcilers.NewServiceEffectiveIntentsReconciler(s.Client,
		s.scheme, epReconciler)

	epReconciler.InjectableRecorder.Recorder = s.Recorder
	s.EPIntentsReconciler.Recorder = s.Recorder
	s.externalNetpolHandler.EXPECT().HandleAllPods(gomock.Any()).AnyTimes()
}

func (s *RulesBuilderTestSuiteBase) TearDownTest() {
	viper.Reset()
	s.externalNetpolHandler = nil
	s.EPIntentsReconciler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *RulesBuilderTestSuiteBase) ignoreRemoveDeprecatedPolicies() {
	deprecatedLabels := []string{otterizev2alpha1.OtterizeEgressNetworkPolicy, otterizev2alpha1.OtterizeSvcEgressNetworkPolicy, otterizev2alpha1.OtterizeInternetNetworkPolicy, otterizev2alpha1.OtterizeSvcNetworkPolicy}
	for _, label := range deprecatedLabels {
		selectorRequirement := metav1.LabelSelectorRequirement{
			Key:      label,
			Operator: metav1.LabelSelectorOpExists,
		}
		selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
			selectorRequirement,
		}})

		s.Require().NoError(err)

		s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&v1.NetworkPolicyList{}), &client.ListOptions{LabelSelector: selector}).Return(nil).AnyTimes()
	}
}

func (s *RulesBuilderTestSuiteBase) ignoreRemoveOrphan() {
	s.expectRemoveOrphanFindsPolicies(nil)
}
func (s *RulesBuilderTestSuiteBase) expectRemoveOrphanFindsPolicies(netpols []v1.NetworkPolicy) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
		{
			Key:      otterizev2alpha1.OtterizeNetworkPolicy,
			Operator: metav1.LabelSelectorOpExists,
		},
		{
			Key:      otterizev2alpha1.OtterizeNetworkPolicyExternalTraffic,
			Operator: metav1.LabelSelectorOpDoesNotExist,
		},
		{
			Key:      otterizev2alpha1.OtterizeNetworkPolicyServiceDefaultDeny,
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
	s.ignoreRemoveDeprecatedPolicies()
}

func (s *RulesBuilderTestSuiteBase) expectGetAllEffectivePolicies(clientIntents []otterizev2alpha1.ClientIntents) {
	var intentsList otterizev2alpha1.ClientIntentsList
	for _, clientIntentsEntry := range clientIntents {
		s.expectKubernetesServicesReferencingPodsIndirectly(clientIntentsEntry)
	}

	s.Client.EXPECT().List(gomock.Any(), &intentsList).DoAndReturn(func(_ context.Context, intents *otterizev2alpha1.ClientIntentsList, _ ...any) error {
		intents.Items = append(intents.Items, clientIntents...)
		return nil
	})

	var podList corev1.PodList
	s.Client.EXPECT().List(gomock.Any(), &podList, client.MatchingFields{otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationIndexField: otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationValue}).DoAndReturn(
		func(_ context.Context, pods *corev1.PodList, _ ...any) error {
			return nil
		})

	// create service to ClientIntents pointing to it
	services := make(map[string][]otterizev2alpha1.ClientIntents)
	for _, clientIntent := range clientIntents {
		for _, intentCall := range clientIntent.GetTargetList() {
			serverService := intentCall.ToServiceIdentity(clientIntent.Namespace)
			services[serverService.GetFormattedOtterizeIdentityWithKind()] = append(services[serverService.GetFormattedOtterizeIdentityWithoutKind()], clientIntent)
		}
	}

	s.Client.EXPECT().List(
		gomock.Any(),
		&otterizev2alpha1.ClientIntentsList{},
		gomock.AssignableToTypeOf(&client.MatchingFields{}),
	).DoAndReturn(func(_ context.Context, intents *otterizev2alpha1.ClientIntentsList, args ...any) error {
		matchFields := args[0].(*client.MatchingFields)
		intents.Items = services[(*matchFields)[otterizev2alpha1.OtterizeFormattedTargetServerIndexField]]
		return nil
	}).AnyTimes()
}

func (s *RulesBuilderTestSuiteBase) addExpectedKubernetesServiceCall(serviceName string, serviceNamespace string, ports []corev1.ServicePort, selector map[string]string) *corev1.Service {
	serverStrippedSVCPrefix := strings.ReplaceAll(serviceName, "svc:", "")
	kubernetesSvcNamespacedName := types.NamespacedName{
		Namespace: serviceNamespace,
		Name:      serverStrippedSVCPrefix,
	}
	svcObject := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverStrippedSVCPrefix,
			Namespace: serviceNamespace,
		},

		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports:    ports,
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), kubernetesSvcNamespacedName, gomock.AssignableToTypeOf(&svcObject)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, service *corev1.Service, options ...client.ListOption) error {
			svcObject.DeepCopyInto(service)
			return nil
		}).AnyTimes()

	return &svcObject
}

func (s *RulesBuilderTestSuiteBase) expectKubernetesServicesReferencingPodsIndirectly(clientIntents otterizev2alpha1.ClientIntents) {
	podList := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server",
				Namespace: testClientNamespace,
				Labels: map[string]string{
					testServerServiceLabelKey: testServerServiceLabelValue,
				},
			},
		},
	}
	for _, target := range clientIntents.Spec.Targets {
		if target.GetTargetServerKind() == serviceidentity.KindOtterizeLegacy {
			s.Client.EXPECT().List(
				gomock.Any(),
				&corev1.PodList{},
				client.MatchingLabels{otterizev2alpha1.OtterizeServiceLabelKey: target.ToServiceIdentity(clientIntents.Namespace).GetFormattedOtterizeIdentityWithoutKind()},
			).DoAndReturn(func(ctx context.Context, outPodList *corev1.PodList, _ client.ListOption) error {
				outPodList.Items = podList
				return nil
			}).AnyTimes()
		} else {
			s.Client.EXPECT().List(
				gomock.Any(),
				&corev1.PodList{},
				client.MatchingLabels{otterizev2alpha1.OtterizeServiceLabelKey: target.ToServiceIdentity(clientIntents.Namespace).GetFormattedOtterizeIdentityWithoutKind(),
					otterizev2alpha1.OtterizeOwnerKindLabelKey: target.GetTargetServerKind()}).DoAndReturn(func(ctx context.Context, outPodList *corev1.PodList, _ client.ListOption) error {
				outPodList.Items = podList
				return nil
			}).AnyTimes()
		}

		// Expect service list
		serviceList := []corev1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: testClientNamespace,
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{
						testServerServiceLabelKey: testServerServiceLabelValue,
					},
				},
			},
		}
		var emptyServiceList corev1.ServiceList
		s.Client.EXPECT().List(
			gomock.Any(),
			&emptyServiceList,
			client.InNamespace(testClientNamespace),
		).DoAndReturn(func(ctx context.Context, outServiceList *corev1.ServiceList, _ client.ListOption) error {
			outServiceList.Items = serviceList
			return nil
		}).AnyTimes()
	}
}
