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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
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
	policyName := "authorization-policy-to-test-server-from-test-client.test-namespace"
	clientIntentsNamespace := "test-namespace"

	intents := &v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-test-namespace-8ddecb",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
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
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-test-namespace-8ddecb",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
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

	s.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(client.MatchingLabels{})).Return(nil)
	s.mockClient.EXPECT().Create(gomock.Any(), newPolicy).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonCreatedIstioPolicy)
}

func (s *CreatorTestSuite) TestCreateHTTPResources() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client.test-namespace"
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
					Type: v1alpha2.IntentTypeHTTP,
					HTTPResources: []v1alpha2.HTTPResource{
						{
							Path: "/login",
							Methods: []v1alpha2.HTTPMethod{
								v1alpha2.HTTPMethodGet,
								v1alpha2.HTTPMethodPost,
							},
						},
						{
							Path: "/logout",
							Methods: []v1alpha2.HTTPMethod{
								v1alpha2.HTTPMethodPost,
							},
						},
					},
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
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-test-namespace-8ddecb",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: "test-server-test-namespace-8ddecb",
				},
			},
			Rules: []*v1beta12.Rule{
				{
					To: []*v1beta12.Rule_To{
						{
							Operation: &v1beta12.Operation{
								Paths: []string{
									"/login",
								},
								Methods: []string{
									"GET",
									"POST",
								},
							},
						},
						{
							Operation: &v1beta12.Operation{
								Paths: []string{
									"/logout",
								},
								Methods: []string{
									"POST",
								},
							},
						},
					},
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
	s.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(client.MatchingLabels{})).Return(nil)
	s.mockClient.EXPECT().Create(gomock.Any(), newPolicy).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonCreatedIstioPolicy)
}

func (s *CreatorTestSuite) TestUpdateHTTPResources() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client.test-namespace"
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
					Type: v1alpha2.IntentTypeHTTP,
					HTTPResources: []v1alpha2.HTTPResource{
						{
							Path: "/login",
							Methods: []v1alpha2.HTTPMethod{
								v1alpha2.HTTPMethodGet,
								v1alpha2.HTTPMethodPost,
							},
						},
						{
							Path: "/logout",
							Methods: []v1alpha2.HTTPMethod{
								v1alpha2.HTTPMethodPost,
							},
						},
					},
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
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-test-namespace-8ddecb",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: "test-server-test-namespace-8ddecb",
				},
			},
			Rules: []*v1beta12.Rule{
				{
					To: []*v1beta12.Rule_To{
						{
							Operation: &v1beta12.Operation{
								Paths: []string{
									"/login",
								},
								Methods: []string{
									"GET",
									"POST",
								},
							},
						},
						{
							Operation: &v1beta12.Operation{
								Paths: []string{
									"/logout",
								},
								Methods: []string{
									"POST",
								},
							},
						},
					},
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

	existingPolicyWithoutHTTP := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-test-namespace-8ddecb",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
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
	s.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(_ context.Context, policies *v1beta1.AuthorizationPolicyList, _ ...client.ListOption) {
			policies.Items = append(policies.Items, existingPolicyWithoutHTTP)
		}).Return(nil)

	s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(client.MergeFrom(existingPolicyWithoutHTTP))).Do(
		func(_ context.Context, policy *v1beta1.AuthorizationPolicy, patch client.Patch, _ ...client.PatchOption) {
			s.Equal(newPolicy.Name, policy.Name)
			s.Equal(newPolicy.Namespace, policy.Namespace)
			s.Equal(newPolicy.Spec.Selector, policy.Spec.Selector)
			s.Equal(newPolicy.Spec.Rules, policy.Spec.Rules)
			s.Equal(newPolicy.Labels, policy.Labels)
			s.Equal(newPolicy.Spec.Rules[0].To[0].Operation, policy.Spec.Rules[0].To[0].Operation)
			s.Equal(newPolicy.Spec.Rules[0].To[1].Operation, policy.Spec.Rules[0].To[1].Operation)
		}).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonCreatedIstioPolicy)
}

func (s *CreatorTestSuite) TestNothingToUpdateHTTPResources() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client.test-namespace"
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
					Type: v1alpha2.IntentTypeHTTP,
					HTTPResources: []v1alpha2.HTTPResource{
						{
							Path: "/login",
							Methods: []v1alpha2.HTTPMethod{
								v1alpha2.HTTPMethodGet,
								v1alpha2.HTTPMethodPost,
							},
						},
						{
							Path: "/logout",
							Methods: []v1alpha2.HTTPMethod{
								v1alpha2.HTTPMethodPost,
							},
						},
					},
				},
			},
		},
	}
	clientServiceAccountName := "test-client-sa"

	principal := generatePrincipal(clientIntentsNamespace, clientServiceAccountName)
	existingPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-test-namespace-8ddecb",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: "test-server-test-namespace-8ddecb",
				},
			},
			Rules: []*v1beta12.Rule{
				{
					To: []*v1beta12.Rule_To{
						{
							Operation: &v1beta12.Operation{
								Paths: []string{
									"/login",
								},
								Methods: []string{
									"GET",
									"POST",
								},
							},
						},
						{
							Operation: &v1beta12.Operation{
								Paths: []string{
									"/logout",
								},
								Methods: []string{
									"POST",
								},
							},
						},
					},
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

	s.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(_ context.Context, policies *v1beta1.AuthorizationPolicyList, _ ...client.ListOption) {
			policies.Items = append(policies.Items, existingPolicy)
		}).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonCreatedIstioPolicy)
}

func (s *CreatorTestSuite) TestNamespaceNotAllowed() {
	ctx := context.Background()
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client.test-namespace"
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
	s.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(client.MatchingLabels{})).Return(nil)
	err := s.creator.Create(ctx, intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonNamespaceNotAllowed)
}

func (s *CreatorTestSuite) TestNamespaceAllowed() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client.test-namespace"
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
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-test-namespace-8ddecb",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
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
	s.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(client.MatchingLabels{})).Return(nil)
	s.mockClient.EXPECT().Create(gomock.Any(), newPolicy).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonCreatedIstioPolicy)
}

func (s *CreatorTestSuite) TestUpdatePolicy() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client.test-namespace"
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
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-test-namespace-8ddecb",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
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
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-test-namespace-8ddecb",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
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

	s.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(_ context.Context, policies *v1beta1.AuthorizationPolicyList, _ ...client.ListOption) {
			policies.Items = append(policies.Items, existingPolicy)
		}).Return(nil)

	s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(client.MergeFrom(existingPolicy))).Do(
		func(_ context.Context, policy *v1beta1.AuthorizationPolicy, patch client.Patch, _ ...client.PatchOption) {
			s.Equal(newPolicy.Name, policy.Name)
			s.Equal(newPolicy.Namespace, policy.Namespace)
			s.Equal(newPolicy.Spec.Selector, policy.Spec.Selector)
			s.Equal(newPolicy.Spec.Rules, policy.Spec.Rules)
			s.Equal(newPolicy.Labels, policy.Labels)
		}).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
	s.expectEvent(ReasonCreatedIstioPolicy)
}

func (s *CreatorTestSuite) TestDeleteAllPoliciesForClientIntents() {
	clientName := "test-client"
	serverName1 := "test-server-1"
	serverName2 := "test-server-2"
	clientIntentsNamespace := "test-namespace"

	intents := &v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-client-intents",
			Namespace: clientIntentsNamespace,
		},
		Spec: &v1alpha2.IntentsSpec{
			Service: v1alpha2.Service{
				Name: clientName,
			},
			Calls: []v1alpha2.Intent{
				{
					Name: serverName1,
				},
				{
					Name: serverName2,
				},
			},
		},
	}

	s.mockClient.EXPECT().DeleteAllOf(gomock.Any(), &v1beta1.AuthorizationPolicy{}, client.MatchingLabels{
		v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
	}).Return(nil)

	err := s.creator.DeleteAll(context.Background(), intents)
	s.NoError(err)
}

func (s *CreatorTestSuite) TestNothingToUpdate() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client.test-namespace"
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
	existingPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-test-namespace-8ddecb",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
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

	s.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(_ context.Context, policies *v1beta1.AuthorizationPolicyList, _ ...client.ListOption) {
			policies.Items = append(policies.Items, existingPolicy)
		}).Return(nil)

	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
}

func (s *CreatorTestSuite) TestDeletePolicy() {
	clientName := "test-client"
	serverName := "test-server"
	policyName := "authorization-policy-to-test-server-from-test-client.test-namespace"
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
	existingPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-test-namespace-8ddecb",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
			UID: "uid_1",
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

	outdatedPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntentsNamespace,
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           "test-server-from-old-intent-file",
				v1alpha2.OtterizeIstioClientAnnotationKey: "test-client-test-namespace-537e87",
			},
			UID: "uid_2",
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: "test-server-from-old-intent-file",
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

	s.mockClient.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).Do(
		func(_ context.Context, policies *v1beta1.AuthorizationPolicyList, _ ...client.ListOption) {
			policies.Items = append(policies.Items, outdatedPolicy, existingPolicy)
		}).Return(nil)

	s.mockClient.EXPECT().Delete(gomock.Any(), outdatedPolicy).Return(nil)
	err := s.creator.Create(context.Background(), intents, clientIntentsNamespace, clientServiceAccountName)
	s.NoError(err)
}

func (s *CreatorTestSuite) TestUpdateStatusServiceAccount() {
	clientName := "test-client"
	serverName := "test-server"
	clientIntentsNamespace := "test-namespace"

	intents := &v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-client-intents",
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
	labeledIntents := v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-client-intents",
			Namespace: clientIntentsNamespace,
			Annotations: map[string]string{
				v1alpha2.OtterizeClientServiceAccountAnnotation: clientServiceAccountName,
			},
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

	intentsWithStatus := v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-client-intents",
			Namespace: clientIntentsNamespace,
			Annotations: map[string]string{
				v1alpha2.OtterizeClientServiceAccountAnnotation: clientServiceAccountName,
				v1alpha2.OtterizeSharedServiceAccountAnnotation: "false",
			},
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
	gomock.InOrder(
		s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, intents *v1alpha2.ClientIntents, _ client.Patch, _ ...client.PatchOption) {
			s.Equal(labeledIntents, *intents)
		}).Return(nil),
		s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, intents *v1alpha2.ClientIntents, _ client.Patch, _ ...client.PatchOption) {
			missingSideCar, ok := intents.Annotations[v1alpha2.OtterizeMissingSidecarAnnotation]
			s.True(ok)
			s.Equal(strconv.FormatBool(false), missingSideCar)
		}).Return(nil),
		s.mockClient.EXPECT().List(gomock.Any(), &v1alpha2.ClientIntentsList{}, &client.ListOptions{Namespace: clientIntentsNamespace}).Do(func(_ context.Context, intents *v1alpha2.ClientIntentsList, _ ...client.ListOption) {
			intents.Items = append(intents.Items, labeledIntents)
		}).Return(nil),
		s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, intents *v1alpha2.ClientIntents, _ client.Patch, _ ...client.PatchOption) {
			s.Equal(intentsWithStatus, *intents)
		}).Return(nil),
	)

	err := s.creator.UpdateIntentsStatus(context.Background(), intents, clientServiceAccountName, false)
	s.NoError(err)
	s.expectEvent(ReasonServiceAccountFound)
}

func (s *CreatorTestSuite) TestUpdateStatusMissingSidecar() {
	clientName := "test-client"
	serverName := "test-server"
	clientIntentsNamespace := "test-namespace"

	intents := &v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-client-intents",
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
	labeledIntents := v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-client-intents",
			Namespace: clientIntentsNamespace,
			Annotations: map[string]string{
				v1alpha2.OtterizeClientServiceAccountAnnotation: clientServiceAccountName,
			},
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

	intentsWithStatus := v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-client-intents",
			Namespace: clientIntentsNamespace,
			Annotations: map[string]string{
				v1alpha2.OtterizeClientServiceAccountAnnotation: clientServiceAccountName,
				v1alpha2.OtterizeSharedServiceAccountAnnotation: "false",
			},
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
	gomock.InOrder(
		s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, intents *v1alpha2.ClientIntents, _ client.Patch, _ ...client.PatchOption) {
			s.Equal(labeledIntents, *intents)
		}).Return(nil),
		s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, intents *v1alpha2.ClientIntents, _ client.Patch, _ ...client.PatchOption) {
			missingSideCar, ok := intents.Annotations[v1alpha2.OtterizeMissingSidecarAnnotation]
			s.True(ok)
			s.Equal(strconv.FormatBool(true), missingSideCar)
		}).Return(nil),
		s.mockClient.EXPECT().List(gomock.Any(), &v1alpha2.ClientIntentsList{}, &client.ListOptions{Namespace: clientIntentsNamespace}).Do(func(_ context.Context, intents *v1alpha2.ClientIntentsList, _ ...client.ListOption) {
			intents.Items = append(intents.Items, labeledIntents)
		}).Return(nil),
		s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, intents *v1alpha2.ClientIntents, _ client.Patch, _ ...client.PatchOption) {
			s.Equal(intentsWithStatus, *intents)
		}).Return(nil),
	)

	err := s.creator.UpdateIntentsStatus(context.Background(), intents, clientServiceAccountName, true)
	s.NoError(err)
	s.expectEvent(ReasonServiceAccountFound)
	s.expectEvent(ReasonMissingSidecar)
}

func (s *CreatorTestSuite) TestUpdateStatusSharedServiceAccount() {
	clientName := "test-client"
	serverName := "test-server"
	clientIntentsNamespace := "test-namespace"

	intents := &v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-client-intents",
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
	labeledIntents := v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-client-intents",
			Namespace: clientIntentsNamespace,
			Annotations: map[string]string{
				v1alpha2.OtterizeClientServiceAccountAnnotation: clientServiceAccountName,
			},
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

	anotherIntents := v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-other-client-intents",
			Namespace: clientIntentsNamespace,
			Annotations: map[string]string{
				v1alpha2.OtterizeClientServiceAccountAnnotation: clientServiceAccountName,
			},
		},
		Spec: &v1alpha2.IntentsSpec{
			Service: v1alpha2.Service{
				Name: "another-client",
			},
			Calls: []v1alpha2.Intent{
				{
					Name: "another-server",
				},
			},
		},
	}

	intentsWithStatus := v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-client-intents",
			Namespace: clientIntentsNamespace,
			Annotations: map[string]string{
				v1alpha2.OtterizeClientServiceAccountAnnotation: clientServiceAccountName,
				v1alpha2.OtterizeSharedServiceAccountAnnotation: "true",
			},
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

	anotherIntentsWithStatus := v1alpha2.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test-other-client-intents",
			Namespace: clientIntentsNamespace,
			Annotations: map[string]string{
				v1alpha2.OtterizeClientServiceAccountAnnotation: clientServiceAccountName,
				v1alpha2.OtterizeSharedServiceAccountAnnotation: "true",
			},
		},
		Spec: &v1alpha2.IntentsSpec{
			Service: v1alpha2.Service{
				Name: "another-client",
			},
			Calls: []v1alpha2.Intent{
				{
					Name: "another-server",
				},
			},
		},
	}
	isMissingSideCar := false

	gomock.InOrder(
		s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, intents *v1alpha2.ClientIntents, _ client.Patch, _ ...client.PatchOption) {
			s.Equal(labeledIntents, *intents)
		}).Return(nil),
		s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, intents *v1alpha2.ClientIntents, _ client.Patch, _ ...client.PatchOption) {
			missingSideCar, ok := intents.Annotations[v1alpha2.OtterizeMissingSidecarAnnotation]
			s.Equal(true, ok)
			s.Equal(strconv.FormatBool(isMissingSideCar), missingSideCar)
		}).Return(nil),
		s.mockClient.EXPECT().List(gomock.Any(), &v1alpha2.ClientIntentsList{}, &client.ListOptions{Namespace: clientIntentsNamespace}).Do(func(_ context.Context, intents *v1alpha2.ClientIntentsList, _ ...client.ListOption) {
			intents.Items = append(intents.Items, labeledIntents, anotherIntents)
		}).Return(nil),
	)

	gomock.InOrder(
		s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, intents *v1alpha2.ClientIntents, _ client.Patch, _ ...client.PatchOption) {
			s.Equal(intentsWithStatus, *intents)
		}).Return(nil),
	)

	gomock.InOrder(
		s.mockClient.EXPECT().Patch(gomock.Any(), gomock.Any(), gomock.Any()).Do(func(_ context.Context, intents *v1alpha2.ClientIntents, _ client.Patch, _ ...client.PatchOption) {
			s.Equal(anotherIntentsWithStatus, *intents)
		}).Return(nil),
	)

	err := s.creator.UpdateIntentsStatus(context.Background(), intents, clientServiceAccountName, isMissingSideCar)
	s.NoError(err)
	s.expectEvent(ReasonServiceAccountFound)
	s.expectEvent(ReasonSharedServiceAccount)
	s.expectEvent(ReasonSharedServiceAccount)
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
