package builders

import (
	"context"
	"fmt"
	"github.com/golang/mock/gomock"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type EgressDNSNetworkPolicyReconcilerTestSuite struct {
	RulesBuilderTestSuiteBase
	Builder *DNSEgressNetworkPolicyBuilder
}

func (s *EgressDNSNetworkPolicyReconcilerTestSuite) SetupTest() {
	s.RulesBuilderTestSuiteBase.SetupTest()
	s.Builder = NewDNSEgressNetworkPolicyBuilder()
	s.Builder.Recorder = s.Recorder
	s.Reconciler.AddEgressRuleBuilder(s.Builder)
}

func (s *EgressDNSNetworkPolicyReconcilerTestSuite) TearDownTest() {
	s.RulesBuilderTestSuiteBase.TearDownTest()
	s.Builder = nil
}

func (s *EgressDNSNetworkPolicyReconcilerTestSuite) TestCreateNetworkPolicy() {
	viper.Set(operatorconfig.EnableEgressAutoallowDNSTrafficKey, true)
	defer func() {
		viper.Set(operatorconfig.EnableEgressAutoallowDNSTrafficKey, false)
	}()
	clientIntentsName := "client-intents"
	policyName := "test-client-access"
	serviceName := "test-client"
	serverNamespace := testServerNamespace
	clientNamespace := testClientNamespace
	formattedTargetServer := "test-server-test-server-namespac-48aee4"

	s.testCreateNetworkPolicyDNS(
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

func (s *EgressDNSNetworkPolicyReconcilerTestSuite) testCreateNetworkPolicyDNS(
	clientIntentsName string,
	clientNamespace string,
	serverNamespace string,
	serviceName string,
	policyName string,
	formattedTargetServer string,
	defaultEnforcementState bool,
	protectedServices []otterizev2.ProtectedService,
) {
	s.Reconciler.EnforcementDefaultState = defaultEnforcementState
	namespacedName := types.NamespacedName{
		Namespace: testClientNamespace,
		Name:      clientIntentsName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := fmt.Sprintf("test-server.%s", serverNamespace)
	intentsSpec := &otterizev2.IntentsSpec{
		Workload: otterizev2.Workload{Name: serviceName},
		Targets: []otterizev2.Target{
			{
				Kubernetes: &otterizev2.KubernetesTarget{Name: serverName},
			},
		},
	}

	// Initial call to get the ClientIntents object when reconciler starts
	clientIntents := otterizev2.ClientIntents{Spec: intentsSpec}
	clientIntents.Namespace = clientNamespace
	clientIntents.Name = clientIntentsName

	if defaultEnforcementState == false {
		s.Client.EXPECT().List(gomock.Any(), gomock.Eq(&otterizev2.ProtectedServiceList{}), gomock.Any()).DoAndReturn(
			func(ctx context.Context, list *otterizev2.ProtectedServiceList, opts ...client.ListOption) error {
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
	newPolicy := networkPolicyDNSEgressTemplate(
		policyName,
		serverNamespace,
		"test-client-test-client-namespac-edb3a2",
		formattedTargetServer,
		testClientNamespace,
	)
	s.Client.EXPECT().Create(gomock.Any(), gomock.Eq(newPolicy)).Return(nil)

	s.ignoreRemoveOrphan()

	s.expectGetAllEffectivePolicies([]otterizev2.ClientIntents{clientIntents})
	s.externalNetpolHandler.EXPECT().HandlePodsByLabelSelector(gomock.Any(), gomock.Any(), gomock.Any())
	res, err := s.EPIntentsReconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Empty(res)
}

func networkPolicyDNSEgressTemplate(
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
				otterizev2.OtterizeNetworkPolicy: formattedTargetClient,
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeEgress},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					otterizev2.OtterizeServiceLabelKey: formattedTargetClient,
				},
			},
			Egress: []v1.NetworkPolicyEgressRule{
				{
					Ports: []v1.NetworkPolicyPort{
						{
							Protocol: lo.ToPtr(corev1.ProtocolUDP),
							Port:     lo.ToPtr(intstr.FromInt32(53)),
						},
					},
				},
			},
		},
	}
}

func TestDNSEgressNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(EgressDNSNetworkPolicyReconcilerTestSuite))
}
