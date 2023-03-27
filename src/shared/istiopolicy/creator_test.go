package istiopolicy

import (
	"context"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	istiopolicymocks "github.com/otterize/intents-operator/src/shared/istiopolicy/mocks"
	"github.com/stretchr/testify/suite"
	v1beta12 "istio.io/api/security/v1beta1"
	v1beta13 "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type CreatorTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockClient *istiopolicymocks.MockClient
	recorder   *record.FakeRecorder
	creator    *Creator
}

func (s *CreatorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockClient = istiopolicymocks.NewMockClient(s.ctrl)
	s.recorder = record.NewFakeRecorder(100)
	s.creator = NewCreator(s.mockClient, &injectablerecorder.InjectableRecorder{Recorder: s.recorder}, []string{})
}

func (s *CreatorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CreatorTestSuite) TestCreate() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client"
	clientIntentsNamespace := "test-namespace"

	intents := &v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: &v1alpha2.IntentsSpec{
			Service: v1alpha2.Service{
				Name: clientName,
			},
			Calls: []v1alpha2.Intent{
				{
					Name: serverName,
				},
			},
		},
	}
	clientServiceAccountName := "test-client-sa"

	principal := generatePrincipal(clientIntentsNamespace, clientServiceAccountName)
	newPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: "test-server-test-namespace-8ddecb",
				},
			},
			Rules: []*v1beta12.Rule{
				{
					From: []*v1beta12.Rule_From{
						{
							Source: &v1beta12.Source{
								Principals: []string{
									principal,
								},
							},
						},
					},
				},
			},
		},
	}
	s.mockClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: policyName, Namespace: clientIntentsNamespace}, gomock.Any()).Return(errors.NewNotFound(v1beta1.Resource("authorizationpolicy"), policyName))
	s.mockClient.EXPECT().Create(gomock.Any(), newPolicy).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonCreatedIstioPolicy)
}

func (s *CreatorTestSuite) TestNamespaceNotAllowed() {
	ctx := context.Background()
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client"
	clientIntentsNamespace := "test-namespace"
	s.creator.restrictToNamespaces = []string{fmt.Sprintf("this-is-not-%s", clientIntentsNamespace)}
	intents := &v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: &v1alpha2.IntentsSpec{
			Service: v1alpha2.Service{
				Name: clientName,
			},
			Calls: []v1alpha2.Intent{
				{
					Name: serverName,
				},
			},
		},
	}
	clientServiceAccountName := "test-client-sa"
	err := s.creator.Create(ctx, intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonNamespaceNotAllowed)
}

func (s *CreatorTestSuite) TestNamespaceAllowed() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client"
	clientIntentsNamespace := "test-namespace"
	s.creator.restrictToNamespaces = []string{clientIntentsNamespace}
	intents := &v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: &v1alpha2.IntentsSpec{
			Service: v1alpha2.Service{
				Name: clientName,
			},
			Calls: []v1alpha2.Intent{
				{
					Name: serverName,
				},
			},
		},
	}
	clientServiceAccountName := "test-client-sa"

	principal := generatePrincipal(clientIntentsNamespace, clientServiceAccountName)
	newPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: "test-server-test-namespace-8ddecb",
				},
			},
			Rules: []*v1beta12.Rule{
				{
					From: []*v1beta12.Rule_From{
						{
							Source: &v1beta12.Source{
								Principals: []string{
									principal,
								},
							},
						},
					},
				},
			},
		},
	}
	s.mockClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: policyName, Namespace: clientIntentsNamespace}, gomock.Any()).Return(errors.NewNotFound(v1beta1.Resource("authorizationpolicy"), policyName))
	s.mockClient.EXPECT().Create(gomock.Any(), newPolicy).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonCreatedIstioPolicy)
}

func (s *CreatorTestSuite) TestUpdatePolicy() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client"
	clientIntentsNamespace := "test-namespace"

	intents := &v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: &v1alpha2.IntentsSpec{
			Service: v1alpha2.Service{
				Name: clientName,
			},
			Calls: []v1alpha2.Intent{
				{
					Name: serverName,
				},
			},
		},
	}
	clientServiceAccountName := "test-client-sa"

	principal := generatePrincipal(clientIntentsNamespace, clientServiceAccountName)
	newPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: "test-server-test-namespace-8ddecb",
				},
			},
			Rules: []*v1beta12.Rule{
				{
					From: []*v1beta12.Rule_From{
						{
							Source: &v1beta12.Source{
								Principals: []string{
									principal,
								},
							},
						},
					},
				},
			},
		},
	}

	outDatedServiceAccountPrincipal := generatePrincipal(clientIntentsNamespace, "outdated-service-account")
	existingPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: "test-server-test-namespace-8ddecb",
				},
			},
			Rules: []*v1beta12.Rule{
				{
					From: []*v1beta12.Rule_From{
						{
							Source: &v1beta12.Source{
								Principals: []string{
									outDatedServiceAccountPrincipal,
								},
							},
						},
					},
				},
			},
		},
	}

	s.mockClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: policyName, Namespace: clientIntentsNamespace}, gomock.Any()).Do(
		func(_ context.Context, _ types.NamespacedName, policy *v1beta1.AuthorizationPolicy, _ ...client.GetOption) {
			*policy = *existingPolicy.DeepCopy()
		}).Return(nil)

	s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(client.MergeFrom(existingPolicy))).Do(
		func(_ context.Context, policy *v1beta1.AuthorizationPolicy, patch client.Patch, _ ...client.PatchOption) {
			s.Equal(newPolicy.Name, policy.Name)
			s.Equal(newPolicy.Namespace, policy.Namespace)
			s.Equal(newPolicy.Spec.Selector, policy.Spec.Selector)
			s.Equal(newPolicy.Spec.Rules, policy.Spec.Rules)
		}).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonCreatedIstioPolicy)
}

func (s *CreatorTestSuite) TestOverrideNoSelector() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client"
	clientIntentsNamespace := "test-namespace"

	intents := &v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: &v1alpha2.IntentsSpec{
			Service: v1alpha2.Service{
				Name: clientName,
			},
			Calls: []v1alpha2.Intent{
				{
					Name: serverName,
				},
			},
		},
	}
	clientServiceAccountName := "test-client-sa"

	principal := generatePrincipal(clientIntentsNamespace, clientServiceAccountName)
	newPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: "test-server-test-namespace-8ddecb",
				},
			},
			Rules: []*v1beta12.Rule{
				{
					From: []*v1beta12.Rule_From{
						{
							Source: &v1beta12.Source{
								Principals: []string{
									principal,
								},
							},
						},
					},
				},
			},
		},
	}

	existingPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: "test-server-test-namespace-8ddecb",
				},
			},
		},
	}

	s.mockClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: policyName, Namespace: clientIntentsNamespace}, gomock.Any()).Do(
		func(_ context.Context, _ types.NamespacedName, policy *v1beta1.AuthorizationPolicy, _ ...client.GetOption) {
			*policy = *existingPolicy.DeepCopy()
		}).Return(nil)

	s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(client.MergeFrom(existingPolicy))).Do(
		func(_ context.Context, policy *v1beta1.AuthorizationPolicy, patch client.Patch, _ ...client.PatchOption) {
			s.Equal(newPolicy.Name, policy.Name)
			s.Equal(newPolicy.Namespace, policy.Namespace)
			s.Equal(newPolicy.Spec.Selector, policy.Spec.Selector)
			s.Equal(newPolicy.Spec.Rules, policy.Spec.Rules)
		}).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonCreatedIstioPolicy)
}

func (s *CreatorTestSuite) TestOverrideNoRules() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client"
	clientIntentsNamespace := "test-namespace"

	intents := &v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: &v1alpha2.IntentsSpec{
			Service: v1alpha2.Service{
				Name: clientName,
			},
			Calls: []v1alpha2.Intent{
				{
					Name: serverName,
				},
			},
		},
	}
	clientServiceAccountName := "test-client-sa"

	principal := generatePrincipal(clientIntentsNamespace, clientServiceAccountName)
	newPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: "test-server-test-namespace-8ddecb",
				},
			},
			Rules: []*v1beta12.Rule{
				{
					From: []*v1beta12.Rule_From{
						{
							Source: &v1beta12.Source{
								Principals: []string{
									principal,
								},
							},
						},
					},
				},
			},
		},
	}

	outDatedServiceAccountPrincipal := generatePrincipal(clientIntentsNamespace, "outdated-service-account")
	existingPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
		},
		Spec: v1beta12.AuthorizationPolicy{
			Rules: []*v1beta12.Rule{
				{
					From: []*v1beta12.Rule_From{
						{
							Source: &v1beta12.Source{
								Principals: []string{
									outDatedServiceAccountPrincipal,
								},
							},
						},
					},
				},
			},
		},
	}

	s.mockClient.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: policyName, Namespace: clientIntentsNamespace}, gomock.Any()).Do(
		func(_ context.Context, _ types.NamespacedName, policy *v1beta1.AuthorizationPolicy, _ ...client.GetOption) {
			*policy = *existingPolicy.DeepCopy()
		}).Return(nil)

	s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(client.MergeFrom(existingPolicy))).Do(
		func(_ context.Context, policy *v1beta1.AuthorizationPolicy, patch client.Patch, _ ...client.PatchOption) {
			s.Equal(newPolicy.Name, policy.Name)
			s.Equal(newPolicy.Namespace, policy.Namespace)
			s.Equal(newPolicy.Spec.Selector, policy.Spec.Selector)
			s.Equal(newPolicy.Spec.Rules, policy.Spec.Rules)
		}).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonCreatedIstioPolicy)
}

func generatePrincipal(clientIntentsNamespace string, clientServiceAccountName string) string {
	return fmt.Sprintf("cluster.local/ns/%s/sa/%s", clientIntentsNamespace, clientServiceAccountName)
}

func (s *CreatorTestSuite) expectEvent(expectedEvent string) {
	select {
	case event := <-s.recorder.Events:
		s.Require().Contains(event, expectedEvent)
	default:
		s.Fail("Expected event not found")
	}
}

func TestCreatorTestSuite(t *testing.T) {
	suite.Run(t, new(CreatorTestSuite))
}
