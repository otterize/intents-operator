package builders

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
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
	testNamespace       = "test-namespace"
	testServerNamespace = "test-server-namespace"
	testClientNamespace = "test-client-namespace"
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
		true,
		true,
		nil,
		nil)
	s.Reconciler.Recorder = s.Recorder

	epReconciler := effectivepolicy.NewGroupReconciler(s.Client,
		s.scheme, s.Reconciler)
	s.EPIntentsReconciler = intents_reconcilers.NewServiceEffectiveIntentsReconciler(s.Client,
		s.scheme, epReconciler)

	epReconciler.InjectableRecorder.Recorder = s.Recorder
	s.EPIntentsReconciler.Recorder = s.Recorder
}

func (s *RulesBuilderTestSuiteBase) TearDownTest() {
	viper.Reset()
	s.externalNetpolHandler = nil
	s.EPIntentsReconciler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *RulesBuilderTestSuiteBase) ignoreRemoveDeprecatedPolicies() {
	deprecatedLabels := []string{otterizev1alpha3.OtterizeEgressNetworkPolicy, otterizev1alpha3.OtterizeSvcEgressNetworkPolicy, otterizev1alpha3.OtterizeInternetNetworkPolicy, otterizev1alpha3.OtterizeSvcNetworkPolicy}
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
	s.ignoreRemoveDeprecatedPolicies()
}

func (s *RulesBuilderTestSuiteBase) expectGetAllEffectivePolicies(clientIntents []otterizev1alpha3.ClientIntents) {
	var intentsList otterizev1alpha3.ClientIntentsList

	s.Client.EXPECT().List(gomock.Any(), &intentsList).DoAndReturn(func(_ context.Context, intents *otterizev1alpha3.ClientIntentsList, _ ...any) error {
		intents.Items = append(intents.Items, clientIntents...)
		return nil
	})

	// create service to ClientIntents pointing to it
	services := make(map[string][]otterizev1alpha3.ClientIntents)
	for _, clientIntent := range clientIntents {
		for _, intentCall := range clientIntent.GetCallsList() {
			serverService := serviceidentity.NewFromIntent(intentCall, clientIntent.Namespace)
			services[serverService.GetFormattedOtterizeIdentity()] = append(services[serverService.GetFormattedOtterizeIdentity()], clientIntent)
		}
	}

	matchFieldsPtr := &client.MatchingFields{}
	s.Client.EXPECT().List(
		gomock.Any(),
		&otterizev1alpha3.ClientIntentsList{},
		gomock.AssignableToTypeOf(matchFieldsPtr),
	).DoAndReturn(func(_ context.Context, intents *otterizev1alpha3.ClientIntentsList, args ...any) error {
		matchFields := args[0].(*client.MatchingFields)
		intents.Items = services[(*matchFields)[otterizev1alpha3.OtterizeFormattedTargetServerIndexField]]
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
