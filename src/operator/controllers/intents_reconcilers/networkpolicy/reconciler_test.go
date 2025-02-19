package networkpolicy

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type NetworkPolicyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
}

func (s *NetworkPolicyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
}

func (s *NetworkPolicyReconcilerTestSuite) TearDownTest() {
	s.MocksSuiteBase.TearDownTest()
}

func (s *NetworkPolicyReconcilerTestSuite) TestAddingWarningForInternetIntentWhenEgressPolicyIsDisabled() {
	networkPolicyReconciler := NewReconciler(
		s.Client,
		scheme.Scheme,
		nil,
		nil,
		nil,
		true,
		true,
		true,
		nil,
		nil, // This means that the egress policy is disabled
	)

	calls := []effectivepolicy.Call{{
		Target: otterizev2alpha1.Target{
			Internet: &otterizev2alpha1.Internet{
				Domains: []string{"example.com"},
			},
		},
		ReferencingKubernetesServices: nil,
		EventRecorder:                 nil,
	}}

	effectivePolicies := []effectivepolicy.ServiceEffectivePolicy{{
		Service: serviceidentity.ServiceIdentity{
			Name:        "test-service",
			Namespace:   "test-namespace",
			Kind:        "Service",
			OwnerObject: nil,
		},
		CalledBy:            nil,
		Calls:               calls,
		ClientIntentsStatus: v2alpha1.ApprovedClientIntentsStatus{},
		ClientIntentsEventRecorder: injectablerecorder.NewObjectEventRecorder(
			&injectablerecorder.InjectableRecorder{Recorder: s.Recorder},
			nil),
	}}

	s.Client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, list *v1.NetworkPolicyList, opts ...client.ListOption) error {
			return nil
		}).AnyTimes()

	_, errs := networkPolicyReconciler.ReconcileEffectivePolicies(context.Background(), effectivePolicies)
	for _, err := range errs {
		s.NoError(err)
	}
	s.ExpectEventsForRecorder(s.Recorder, consts.ReasonInternetEgressNetworkPolicyWithEgressPolicyDisabled)

}

func TestNetworkPolicyReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyReconcilerTestSuite))
}
