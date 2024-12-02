package linkerdmanager

import (
	"context"
	"fmt"
	authpolicy "github.com/linkerd/linkerd2/controller/gen/apis/policy/v1alpha1"
	linkerdserver "github.com/linkerd/linkerd2/controller/gen/apis/server/v1beta1"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	serviceidresolvermocks "github.com/otterize/intents-operator/src/shared/serviceidresolver/mocks"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/stretchr/testify/suite"
	"testing"

	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayapiv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

type policyWrapper struct {
	authpolicy.AuthorizationPolicy
}

type routeWrapper struct {
	authpolicy.HTTPRoute
}

func (rw routeWrapper) String() string {
	return fmt.Sprintf("Namespace: %s, Path.Value: %s, ParentRef.Kind: %s, ParentRef.Name: %s", rw.Namespace, *rw.Spec.Rules[0].Matches[0].Path.Value, *rw.Spec.ParentRefs[0].Kind, rw.Spec.ParentRefs[0].Name)
}

func (rw routeWrapper) Matches(x interface{}) bool {
	valueAsRoute, ok := x.(*authpolicy.HTTPRoute)
	if !ok {
		fmt.Println("value passed cannot be casted into a httproute")
		return false
	}

	if rw.Namespace != valueAsRoute.Namespace {
		return false
	}

	if string(*rw.Spec.Rules[0].Matches[0].Path.Value) != string(*valueAsRoute.Spec.Rules[0].Matches[0].Path.Value) {
		return false
	}
	if string(*rw.Spec.ParentRefs[0].Kind) != string(*valueAsRoute.Spec.ParentRefs[0].Kind) {
		return false
	}
	if string(rw.Spec.ParentRefs[0].Name) != string(valueAsRoute.Spec.ParentRefs[0].Name) {
		return false
	}

	return true
}

func (pw policyWrapper) String() string {
	return ""
	//return fmt.Sprintf("Namespace: %s, TargetRef.Kind: %s, RequiredAuthenticationRef.Name: %s, RequiredAuthenticationRef.Kind: %s", pw.Namespace, pw.Spec.TargetRef.Kind, pw.Spec.RequiredAuthenticationRefs[0].Name, pw.Spec.RequiredAuthenticationRefs[0].Kind)
}

func (pw policyWrapper) Matches(x interface{}) bool {
	valueAsPolicy, ok := x.(*authpolicy.AuthorizationPolicy)
	if !ok {
		fmt.Println("value passed cannot be casted into an authorization policy")
		return false
	}

	if pw.Namespace != valueAsPolicy.Namespace {
		return false
	}

	if string(pw.Spec.TargetRef.Kind) != string(valueAsPolicy.Spec.TargetRef.Kind) {
		return false
	}

	if string(pw.Spec.RequiredAuthenticationRefs[0].Name) != string(valueAsPolicy.Spec.RequiredAuthenticationRefs[0].Name) {
		return false
	}

	if string(pw.Spec.RequiredAuthenticationRefs[0].Kind) != string(valueAsPolicy.Spec.RequiredAuthenticationRefs[0].Kind) {
		return false
	}
	return true
}

type LinkerdManagerTestSuite struct {
	testbase.MocksSuiteBase
	admin                 *LinkerdManager
	mockServiceIDResolver *serviceidresolvermocks.MockServiceResolver
}

func (s *LinkerdManagerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.admin = NewLinkerdManager(s.Client, []string{}, &injectablerecorder.InjectableRecorder{Recorder: s.Recorder}, true)
	s.mockServiceIDResolver = serviceidresolvermocks.NewMockServiceResolver(s.Controller)
}

func (s *LinkerdManagerTestSuite) TearDownTest() {
	s.admin = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *LinkerdManagerTestSuite) TestCreateResourcesNonHTTPIntent() {
	ns := "test-namespace"
	clientServiceName := "service-that-calls"
	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: clientServiceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{Name: "test-service"},
			},
		},
	}

	intents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-intents-object",
			Namespace: ns,
		},
		Spec: intentsSpec,
	}

	podSelector := s.admin.BuildPodLabelSelectorFromTarget(intents.Spec.Targets[0], intents.Namespace)
	svcIdentity := intents.ToServiceIdentity()

	pod := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			//Labels: map[string]string{
			//	otterizev2alpha1.OtterizeServerLabelKey: formattedTargetServer,
			//},
			Name:      "example-pod",
			Namespace: ns,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Ports: []v1.ContainerPort{
						{
							ContainerPort: 8000,
						},
					},
				},
			},
		},
	}
	netAuth := &authpolicy.NetworkAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NetworkAuthenticationNameTemplate,
			Namespace: intents.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
			},
		},
		Spec: authpolicy.NetworkAuthenticationSpec{
			Networks: []*authpolicy.Network{
				{
					Cidr: "0.0.0.0/0",
				},
				{
					Cidr: "::0",
				},
			},
		},
	}

	mtlsAuth := &authpolicy.MeshTLSAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, intents.Spec.Workload.Name),
			Namespace: intents.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
			},
		},
		Spec: authpolicy.MeshTLSAuthenticationSpec{
			Identities: []string{"default.test-namespace.serviceaccount.identity.linkerd.cluster.local"},
		},
	}

	server := &linkerdserver.Server{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "server-for-test-service-port-8000",
			Namespace: ns,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
			},
		},
		Spec: linkerdserver.ServerSpec{
			PodSelector: &podSelector,
			Port:        intstr.FromInt32(8000),
		},
	}

	policy := &authpolicy.AuthorizationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "authpolicy-to-test-service-port-8000-from-client-service-that-calls-knng3afe",
			Namespace: intents.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
			},
		},
		Spec: authpolicy.AuthorizationPolicySpec{
			TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
				Group: "policy.linkerd.io",
				Kind:  v1beta1.Kind("Server"),
				Name:  v1beta1.ObjectName("server-for-test-service-port-8000"),
			},
			RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
				{
					Group: "policy.linkerd.io",
					Kind:  v1beta1.Kind("MeshTLSAuthentication"),
					Name:  "meshtls-for-client-service-that-calls",
				},
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&v1.PodList{}), gomock.Any(),
		gomock.Any()).Do(func(_ any, podList *v1.PodList, _ ...any) {
		podList.Items = append(podList.Items, pod)
	}).Return(nil).AnyTimes()

	s.Client.EXPECT().List(gomock.Any(), &authpolicy.NetworkAuthenticationList{}, gomock.Any()).AnyTimes()
	s.Client.EXPECT().List(gomock.Any(), &authpolicy.MeshTLSAuthenticationList{}, gomock.Any()).AnyTimes()
	s.Client.EXPECT().List(gomock.Any(), &authpolicy.AuthorizationPolicyList{}, gomock.Any()).AnyTimes()
	s.Client.EXPECT().List(gomock.Any(), &linkerdserver.ServerList{}, gomock.Any()).AnyTimes()

	//s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
	s.Client.EXPECT().Create(gomock.Any(), netAuth).Return(nil)
	//s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
	s.Client.EXPECT().Create(gomock.Any(), mtlsAuth).Return(nil)
	//s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
	s.Client.EXPECT().Create(gomock.Any(), server)
	//s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
	s.Client.EXPECT().Create(gomock.Any(), policyWrapper{*policy})

	_, err := s.admin.CreateResources(context.Background(), effectivepolicy.ServiceEffectivePolicy{
		ClientIntentsEventRecorder: injectablerecorder.NewObjectEventRecorder(
			&injectablerecorder.InjectableRecorder{Recorder: s.Recorder},
			&intents),
		Service: serviceidentity.ServiceIdentity{
			Name:      clientServiceName,
			Namespace: ns,
		},
		Calls: []effectivepolicy.Call{
			{
				Target: otterizev2alpha1.Target{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: "test-service",
					},
				},
			},
		},
	}, "default")
	cleanEvents(s.Recorder.Events)
	s.NoError(err)
}

func (s *LinkerdManagerTestSuite) TestCreateResourcesHTTPIntent() {
	ns := "test-namespace"
	clientServiceName := "service-that-calls"

	intentsSpec := &otterizev2alpha1.IntentsSpec{
		Workload: otterizev2alpha1.Workload{Name: clientServiceName},
		Targets: []otterizev2alpha1.Target{
			{
				Kubernetes: &otterizev2alpha1.KubernetesTarget{
					Name: "test-service",
					HTTP: []otterizev2alpha1.HTTPTarget{
						{
							Path:    "/api",
							Methods: nil,
						},
					},
				},
			},
		},
	}

	intents := otterizev2alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-intents-object",
			Namespace: ns,
		},
		Spec: intentsSpec,
	}

	podSelector := s.admin.BuildPodLabelSelectorFromTarget(intents.Spec.Targets[0], intents.Namespace)
	svcIdentity := intents.ToServiceIdentity()

	pod := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				//otterizev2alpha1.OtterizeServiceLabelKey:,
			},
			Name:      "example-pod",
			Namespace: ns,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Ports: []v1.ContainerPort{
						{
							ContainerPort: 8000,
						},
					},
				},
			},
		},
	}

	netAuth := &authpolicy.NetworkAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NetworkAuthenticationNameTemplate,
			Namespace: intents.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
			},
		},
		Spec: authpolicy.NetworkAuthenticationSpec{
			Networks: []*authpolicy.Network{
				{
					Cidr: "0.0.0.0/0",
				},
				{
					Cidr: "::0",
				},
			},
		},
	}

	mtlsAuth := &authpolicy.MeshTLSAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, clientServiceName),
			Namespace: intents.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
			},
		},
		Spec: authpolicy.MeshTLSAuthenticationSpec{
			Identities: []string{"default.test-namespace.serviceaccount.identity.linkerd.cluster.local"},
		},
	}

	server := &linkerdserver.Server{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "server-for-test-service-port-8000",
			Namespace: ns,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
			},
		},
		Spec: linkerdserver.ServerSpec{
			PodSelector: &podSelector,
			Port:        intstr.FromInt32(8000),
		},
	}

	route := &authpolicy.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "http-route-for-test-service-port-8000-knng3afe",
			Namespace: ns,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
			},
		},
		Spec: authpolicy.HTTPRouteSpec{
			CommonRouteSpec: v1beta1.CommonRouteSpec{
				ParentRefs: []v1beta1.ParentReference{
					{
						Group: (*v1beta1.Group)(StringPtr("policy.linkerd.io")),
						Kind:  (*v1beta1.Kind)(StringPtr("Server")),
						Name:  v1beta1.ObjectName("server-for-test-service-port-8000"),
					},
				},
			},
			Rules: []authpolicy.HTTPRouteRule{
				{
					Matches: []authpolicy.HTTPRouteMatch{
						{
							Path: &authpolicy.HTTPPathMatch{
								Type:  getPathMatchPointer(authpolicy.PathMatchPathPrefix),
								Value: StringPtr("/api"),
							},
						},
					},
				},
			},
		},
	}

	policy := &authpolicy.AuthorizationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "some-policy",
			Namespace: ns,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
			},
		},
		Spec: authpolicy.AuthorizationPolicySpec{
			TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
				Group: "policy.linkerd.io",
				Kind:  v1beta1.Kind("HTTPRoute"),
				Name:  v1beta1.ObjectName("http-route-for-test-service-port-8000-knng3afe"),
			},
			RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
				{
					Group: "policy.linkerd.io",
					Kind:  v1beta1.Kind("MeshTLSAuthentication"),
					Name:  "meshtls-for-client-service-that-calls",
				},
			},
		},
	}

	s.Client.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&v1.PodList{}), gomock.Any(),
		gomock.Any()).Do(func(_ any, podList *v1.PodList, _ ...any) {
		podList.Items = append(podList.Items, pod)
	}).Return(nil).AnyTimes()

	s.Client.EXPECT().List(gomock.Any(), &authpolicy.NetworkAuthenticationList{}, gomock.Any()).AnyTimes()
	s.Client.EXPECT().List(gomock.Any(), &authpolicy.MeshTLSAuthenticationList{}, gomock.Any()).AnyTimes()
	s.Client.EXPECT().List(gomock.Any(), &authpolicy.AuthorizationPolicyList{}, gomock.Any()).AnyTimes()
	s.Client.EXPECT().List(gomock.Any(), &authpolicy.HTTPRouteList{}, gomock.Any()).AnyTimes()
	s.Client.EXPECT().List(gomock.Any(), &linkerdserver.ServerList{}, gomock.Any()).AnyTimes()

	s.Client.EXPECT().Create(gomock.Any(), netAuth)
	s.Client.EXPECT().Create(gomock.Any(), mtlsAuth)
	s.Client.EXPECT().Create(gomock.Any(), server)
	s.Client.EXPECT().Create(gomock.Any(), route)
	s.Client.EXPECT().Create(gomock.Any(), policyWrapper{*policy})

	_, err := s.admin.CreateResources(context.Background(), effectivepolicy.ServiceEffectivePolicy{
		ClientIntentsEventRecorder: injectablerecorder.NewObjectEventRecorder(
			&injectablerecorder.InjectableRecorder{Recorder: s.Recorder},
			&intents),
		Service: serviceidentity.ServiceIdentity{
			Name:      clientServiceName,
			Namespace: ns,
		},
		Calls: []effectivepolicy.Call{
			{
				Target: otterizev2alpha1.Target{
					Kubernetes: &otterizev2alpha1.KubernetesTarget{
						Name: "test-service",
					},
				},
			},
		},
	}, "default")
	cleanEvents(s.Recorder.Events)
	s.NoError(err)
}

//	func (s *LinkerdManagerTestSuite) TestDeleteAll() {
//		ns := "test-namespace"
//
//		intents := otterizev2alpha1.ClientIntents{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "intents-object",
//				Namespace: ns,
//			},
//			Spec: &otterizev2alpha1.IntentsSpec{
//				Workload: otterizev2alpha1.Workload{Name: "service-that-calls"},
//				Targets: []otterizev2alpha1.Target{
//					{
//						Name: "test-service",
//					},
//				},
//			},
//		}
//		clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(intents.Spec.Service.Name, intents.Namespace)
//		podSelector := s.admin.BuildPodLabelSelectorFromTarget(intents.Spec.Targets[0], intents.Namespace)
//
//		server := &linkerdserver.Server{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1beta1",
//				Kind:       "Server",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "server-for-test-service-port-8000",
//				Namespace: ns,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity,
//				},
//			},
//			Spec: linkerdserver.ServerSpec{
//				PodSelector: &podSelector,
//				Port:        intstr.FromInt32(8000),
//			},
//		}
//
//		policy := &authpolicy.AuthorizationPolicy{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1alpha1",
//				Kind:       "AuthorizationPolicy",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "some-policy",
//				Namespace: intents.Namespace,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.AuthorizationPolicySpec{
//				TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
//					Group: "policy.linkerd.io",
//					Kind:  v1beta1.Kind("HTTPRoute"),
//					Name:  v1beta1.ObjectName("some-route"),
//				},
//				RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
//					{
//						Group: "policy.linkerd.io",
//						Kind:  v1beta1.Kind("MeshTLSAuthentication"),
//						Name:  "meshtls-for-client-service-that-calls",
//					},
//				},
//			},
//		}
//
//		route := &authpolicy.HTTPRoute{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1beta3",
//				Kind:       "HTTPRoute",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "some-route",
//				Namespace: intents.Namespace,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.HTTPRouteSpec{
//				CommonRouteSpec: v1beta1.CommonRouteSpec{
//					ParentRefs: []v1beta1.ParentReference{
//						{
//							Group: (*v1beta1.Group)(StringPtr("policy.linkerd.io")),
//							Kind:  (*v1beta1.Kind)(StringPtr("Server")),
//							Name:  v1beta1.ObjectName("server-for-test-service-port-6969"),
//						},
//					},
//				},
//			},
//		}
//		netAuth := &authpolicy.NetworkAuthentication{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1alpha1",
//				Kind:       "NetworkAuthentication",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      NetworkAuthenticationNameTemplate,
//				Namespace: intents.Namespace,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.NetworkAuthenticationSpec{
//				Networks: []*authpolicy.Network{
//					{
//						Cidr: "0.0.0.0/0",
//					},
//					{
//						Cidr: "::0",
//					},
//				},
//			},
//		}
//
//		mtlsAuth := &authpolicy.MeshTLSAuthentication{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1alpha1",
//				Kind:       "MeshTLSAuthentication",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, intents.Spec.Service.Name),
//				Namespace: intents.Namespace,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.MeshTLSAuthenticationSpec{
//				Identities: []string{"default.test-namespace.serviceaccount.identity.linkerd.cluster.local"},
//			},
//		}
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(),
//			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity}).Do(func(_ context.Context, authPolicies *authpolicy.AuthorizationPolicyList, _ ...client.ListOption) {
//			authPolicies.Items = append(authPolicies.Items, *policy)
//		}).Return(nil)
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(),
//			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity}).Do(func(_ context.Context, servers *linkerdserver.ServerList, _ ...client.ListOption) {
//			servers.Items = append(servers.Items, *server)
//		}).Return(nil)
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(),
//			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity}).Do(func(_ context.Context, routes *authpolicy.HTTPRouteList, _ ...client.ListOption) {
//			routes.Items = append(routes.Items, *route)
//		}).Return(nil)
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(),
//			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity}).Do(func(_ context.Context, netauths *authpolicy.NetworkAuthenticationList, _ ...client.ListOption) {
//			netauths.Items = append(netauths.Items, *netAuth)
//		}).Return(nil)
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(),
//			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity}).Do(func(_ context.Context, mtlsauths *authpolicy.MeshTLSAuthenticationList, _ ...client.ListOption) {
//			mtlsauths.Items = append(mtlsauths.Items, *mtlsAuth)
//		}).Return(nil)
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Delete(gomock.Any(), policy).Return(nil)
//		s.Client.EXPECT().Delete(gomock.Any(), server).Return(nil)
//		s.Client.EXPECT().Delete(gomock.Any(), route).Return(nil)
//		s.Client.EXPECT().Delete(gomock.Any(), netAuth).Return(nil)
//		s.Client.EXPECT().Delete(gomock.Any(), mtlsAuth).Return(nil)
//
//		err := s.admin.DeleteAll(context.Background(), &intents)
//		s.NoError(err)
//	}
//
//	func (s *LinkerdManagerTestSuite) TestShouldntCreateRoute() {
//		ns := "test-namespace"
//		parentServerName := "server-for-test-service-port-8000"
//
//		intentsSpec := &otterizev2alpha1.IntentsSpec{
//			Workload: otterizev2alpha1.Workload{Name: "service-that-calls"},
//			Targets: []otterizev2alpha1.Target{
//				{
//					Name: "test-service",
//				},
//			},
//		}
//
//		intents := otterizev2alpha1.ClientIntents{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "test-intents-object",
//				Namespace: ns,
//			},
//			Spec: intentsSpec,
//		}
//
//		routes := &authpolicy.HTTPRouteList{
//			Items: []authpolicy.HTTPRoute{
//				{
//					TypeMeta: metav1.TypeMeta{
//						APIVersion: "policy.linkerd.io/v1beta1",
//						Kind:       "HTTPRoute",
//					},
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "some-route",
//						Namespace: ns,
//					},
//					Spec: authpolicy.HTTPRouteSpec{
//						CommonRouteSpec: v1beta1.CommonRouteSpec{
//							ParentRefs: []v1beta1.ParentReference{
//								{
//									Group: (*v1beta1.Group)(StringPtr("policy.linkerd.io")),
//									Kind:  (*v1beta1.Kind)(StringPtr("Server")),
//									Name:  v1beta1.ObjectName(parentServerName),
//								},
//							},
//						},
//						Rules: []authpolicy.HTTPRouteRule{
//							{
//								Matches: []authpolicy.HTTPRouteMatch{
//									{
//										Path: &authpolicy.HTTPPathMatch{
//											Type:  getPathMatchPointer(authpolicy.PathMatchPathPrefix),
//											Value: StringPtr("/api"),
//										},
//									},
//								},
//							},
//						},
//					},
//				},
//			},
//		}
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Do(func(_ context.Context, routeList *authpolicy.HTTPRouteList, _ ...client.ListOption) {
//			routeList.Items = append(routeList.Items, routes.Items...)
//		}).Return(nil)
//		_, shouldCreateRoute, err := s.admin.shouldCreateHTTPRoute(context.Background(), intents, "/api", parentServerName)
//		s.Equal(false, shouldCreateRoute)
//		s.NoError(err)
//
// }
//
//	func (s *LinkerdManagerTestSuite) TestShouldntCreatePolicy() {
//		ns := "test-namespace"
//		targetServer := "server-for-test-service-port-8000"
//		mtlsAuthName := "meshtls-for-client-service-that-calls"
//
//		intentsSpec := &otterizev2alpha1.IntentsSpec{
//			Workload: otterizev2alpha1.Workload{Name: "service-that-calls"},
//			Targets: []otterizev2alpha1.Target{
//				{
//					Name: "test-service",
//				},
//			},
//		}
//
//		intents := otterizev2alpha1.ClientIntents{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "test-intents-object",
//				Namespace: ns,
//			},
//			Spec: intentsSpec,
//		}
//
//		policies := &authpolicy.AuthorizationPolicyList{
//			Items: []authpolicy.AuthorizationPolicy{
//				{
//					TypeMeta: metav1.TypeMeta{
//						APIVersion: "policy.linkerd.io/v1beta1",
//						Kind:       "AuthorizationPolicy",
//					},
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "some-route",
//						Namespace: ns,
//					},
//					Spec: authpolicy.AuthorizationPolicySpec{
//						TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
//							Group: "policy.linkerd.io",
//							Kind:  v1beta1.Kind(LinkerdServerKindName),
//							Name:  v1beta1.ObjectName(targetServer),
//						},
//						RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
//							{
//								Group: "policy.linkerd.io",
//								Kind:  v1beta1.Kind(LinkerdMeshTLSAuthenticationKindName),
//								Name:  v1beta1.ObjectName(mtlsAuthName),
//							},
//						},
//					},
//				},
//			},
//		}
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Do(func(_ context.Context, policyList *authpolicy.AuthorizationPolicyList, _ ...client.ListOption) {
//			policyList.Items = append(policyList.Items, policies.Items...)
//		}).Return(nil)
//		_, shouldCreatePolicy, err := s.admin.shouldCreateAuthPolicy(context.Background(), intents, targetServer, LinkerdServerKindName, mtlsAuthName, LinkerdMeshTLSAuthenticationKindName)
//		s.Equal(false, shouldCreatePolicy)
//		s.NoError(err)
//	}
//
//	func (s *LinkerdManagerTestSuite) TestShouldntCreateServer() {
//		ns := "test-namespace"
//		var port int32 = 3000
//
//		intentsSpec := &otterizev2alpha1.IntentsSpec{
//			Workload: otterizev2alpha1.Workload{Name: "service-that-calls"},
//			Targets: []otterizev2alpha1.Target{
//				{
//					Name: "test-service",
//				},
//			},
//		}
//
//		intents := otterizev2alpha1.ClientIntents{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "test-intents-object",
//				Namespace: ns,
//			},
//			Spec: intentsSpec,
//		}
//
//		podSelector := s.admin.BuildPodLabelSelectorFromTarget(intents.Spec.Targets[0], intents.Namespace)
//		serversList := &linkerdserver.ServerList{
//			Items: []linkerdserver.Server{
//				{
//					TypeMeta: metav1.TypeMeta{
//						APIVersion: "policy.linkerd.io/v1beta1",
//						Kind:       "Server",
//					},
//					ObjectMeta: metav1.ObjectMeta{
//						Name:      "server-for-test-service-port-3000",
//						Namespace: ns,
//					},
//					Spec: linkerdserver.ServerSpec{
//						PodSelector: &podSelector,
//						Port:        intstr.FromInt32(port),
//					},
//				},
//			},
//		}
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Do(func(_ context.Context, emptyServersList *linkerdserver.ServerList, _ ...client.ListOption) {
//			emptyServersList.Items = append(emptyServersList.Items, serversList.Items...)
//		}).Return(nil)
//		_, shouldCreateServer, err := s.admin.shouldCreateServer(context.Background(), intents, intents.Spec.Targets[0], port)
//		s.Equal(false, shouldCreateServer)
//		s.NoError(err)
//	}
//
//	func (s *LinkerdManagerTestSuite) TestCreateResourcesForMultiPortPod() {
//		ns := "test-namespace"
//
//		intentsSpec := &otterizev2alpha1.IntentsSpec{
//			Workload: otterizev2alpha1.Workload{Name: "service-that-calls"},
//			Targets: []otterizev2alpha1.Target{
//				{
//					Name: "test-service",
//					Type: "http",
//					HTTPResources: []otterizev2alpha1.HTTPResource{
//						{
//							Path: "/api",
//						},
//					},
//				},
//			},
//		}
//
//		intents := otterizev2alpha1.ClientIntents{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "test-intents-object",
//				Namespace: ns,
//			},
//			Spec: intentsSpec,
//		}
//
//		formattedTargetServer := otterizev2alpha1.GetFormattedOtterizeIdentity(intents.Spec.Targets[0].GetTargetServerName(), ns)
//		linkerdServerServiceFormattedIdentity := otterizev2alpha1.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)
//		podSelector := s.admin.BuildPodLabelSelectorFromTarget(intents.Spec.Targets[0], intents.Namespace)
//
//		pod := v1.Pod{
//			TypeMeta: metav1.TypeMeta{
//				Kind:       "Pod",
//				APIVersion: "v1",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeServerLabelKey: formattedTargetServer,
//				},
//				Name:      "example-pod",
//				Namespace: ns,
//			},
//			Spec: v1.PodSpec{
//				Containers: []v1.Container{
//					{
//						Ports: []v1.ContainerPort{
//							{
//								ContainerPort: 8000,
//							},
//							{
//								ContainerPort: 4000,
//							},
//						},
//					},
//				},
//			},
//		}
//
//		netAuth := &authpolicy.NetworkAuthentication{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1alpha1",
//				Kind:       "NetworkAuthentication",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      NetworkAuthenticationNameTemplate,
//				Namespace: intents.Namespace,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.NetworkAuthenticationSpec{
//				Networks: []*authpolicy.Network{
//					{
//						Cidr: "0.0.0.0/0",
//					},
//					{
//						Cidr: "::0",
//					},
//				},
//			},
//		}
//
//		mtlsAuth := &authpolicy.MeshTLSAuthentication{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1alpha1",
//				Kind:       "MeshTLSAuthentication",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, intents.Spec.Service.Name),
//				Namespace: intents.Namespace,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.MeshTLSAuthenticationSpec{
//				Identities: []string{"default.test-namespace.serviceaccount.identity.linkerd.cluster.local"},
//			},
//		}
//
//		server1 := &linkerdserver.Server{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1beta1",
//				Kind:       "Server",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "server-for-test-service-port-8000",
//				Namespace: ns,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: linkerdserver.ServerSpec{
//				PodSelector: &podSelector,
//				Port:        intstr.FromInt32(8000),
//			},
//		}
//
//		server2 := &linkerdserver.Server{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1beta1",
//				Kind:       "Server",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "server-for-test-service-port-4000",
//				Namespace: ns,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: linkerdserver.ServerSpec{
//				PodSelector: &podSelector,
//				Port:        intstr.FromInt32(4000),
//			},
//		}
//
//		route1 := &authpolicy.HTTPRoute{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1beta3",
//				Kind:       "HTTPRoute",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "http-route-for-test-service-port-8000-knng3afe",
//				Namespace: ns,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.HTTPRouteSpec{
//				CommonRouteSpec: v1beta1.CommonRouteSpec{
//					ParentRefs: []v1beta1.ParentReference{
//						{
//							Group: (*v1beta1.Group)(StringPtr("policy.linkerd.io")),
//							Kind:  (*v1beta1.Kind)(StringPtr("Server")),
//							Name:  v1beta1.ObjectName("server-for-test-service-port-8000"),
//						},
//					},
//				},
//				Rules: []authpolicy.HTTPRouteRule{
//					{
//						Matches: []authpolicy.HTTPRouteMatch{
//							{
//								Path: &authpolicy.HTTPPathMatch{
//									Type:  getPathMatchPointer(authpolicy.PathMatchPathPrefix),
//									Value: StringPtr("/api"),
//								},
//							},
//						},
//					},
//				},
//			},
//		}
//
//		route2 := &authpolicy.HTTPRoute{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1beta3",
//				Kind:       "HTTPRoute",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "http-route-for-test-service-port-8000-knng3afe",
//				Namespace: ns,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.HTTPRouteSpec{
//				CommonRouteSpec: v1beta1.CommonRouteSpec{
//					ParentRefs: []v1beta1.ParentReference{
//						{
//							Group: (*v1beta1.Group)(StringPtr("policy.linkerd.io")),
//							Kind:  (*v1beta1.Kind)(StringPtr("Server")),
//							Name:  v1beta1.ObjectName("server-for-test-service-port-4000"),
//						},
//					},
//				},
//				Rules: []authpolicy.HTTPRouteRule{
//					{
//						Matches: []authpolicy.HTTPRouteMatch{
//							{
//								Path: &authpolicy.HTTPPathMatch{
//									Type:  getPathMatchPointer(authpolicy.PathMatchPathPrefix),
//									Value: StringPtr("/api"),
//								},
//							},
//						},
//					},
//				},
//			},
//		}
//
//		policy1 := &authpolicy.AuthorizationPolicy{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1alpha1",
//				Kind:       "AuthorizationPolicy",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "some-policy",
//				Namespace: ns,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.AuthorizationPolicySpec{
//				TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
//					Group: "policy.linkerd.io",
//					Kind:  v1beta1.Kind("HTTPRoute"),
//					Name:  v1beta1.ObjectName("http-route-for-test-service-port-8000-knng3afe"),
//				},
//				RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
//					{
//						Group: "policy.linkerd.io",
//						Kind:  v1beta1.Kind("MeshTLSAuthentication"),
//						Name:  "meshtls-for-client-service-that-calls",
//					},
//				},
//			},
//		}
//
//		policy2 := &authpolicy.AuthorizationPolicy{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1alpha1",
//				Kind:       "AuthorizationPolicy",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "some-policy",
//				Namespace: ns,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.AuthorizationPolicySpec{
//				TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
//					Group: "policy.linkerd.io",
//					Kind:  v1beta1.Kind("HTTPRoute"),
//					Name:  v1beta1.ObjectName("http-route-for-test-service-port-8000-knng3afe"),
//				},
//				RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
//					{
//						Group: "policy.linkerd.io",
//						Kind:  v1beta1.Kind("MeshTLSAuthentication"),
//						Name:  "meshtls-for-client-service-that-calls",
//					},
//				},
//			},
//		}
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), client.MatchingLabels{otterizev2alpha1.OtterizeServerLabelKey: formattedTargetServer},
//			client.InNamespace(ns)).Do(func(_ context.Context, podList *v1.PodList, _ ...client.ListOption) {
//			podList.Items = append(podList.Items, pod)
//		})
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), netAuth)
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), mtlsAuth)
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), server1)
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), routeWrapper{route1})
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), policyWrapper{policy1})
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), server2)
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), routeWrapper{route2})
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), policyWrapper{policy2})
//
//		_, err := s.admin.CreateResources(context.Background(), &intents, "default")
//		s.NoError(err)
//	}
//
//	func (s *LinkerdManagerTestSuite) TestDeleteOutdatedResources() {
//		ns := "test-namespace"
//
//		intentsSpec := &otterizev2alpha1.IntentsSpec{
//			Workload: otterizev2alpha1.Workload{Name: "service-that-calls"},
//			Targets: []otterizev2alpha1.Target{
//				{
//					Name: "test-service",
//				},
//			},
//		}
//
//		intents := otterizev2alpha1.ClientIntents{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "test-intents-object",
//				Namespace: ns,
//			},
//			Spec: intentsSpec,
//		}
//
//		formattedTargetServer := otterizev2alpha1.GetFormattedOtterizeIdentity(intents.Spec.Targets[0].GetTargetServerName(), ns)
//		linkerdServerServiceFormattedIdentity := otterizev2alpha1.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)
//		podSelector := s.admin.BuildPodLabelSelectorFromTarget(intents.Spec.Targets[0], intents.Namespace)
//
//		pod := v1.Pod{
//			TypeMeta: metav1.TypeMeta{
//				Kind:       "Pod",
//				APIVersion: "v1",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeServerLabelKey: formattedTargetServer,
//				},
//				Name:      "example-pod",
//				Namespace: ns,
//			},
//			Spec: v1.PodSpec{
//				Containers: []v1.Container{
//					{
//						Ports: []v1.ContainerPort{
//							{
//								ContainerPort: 8000,
//							},
//						},
//					},
//				},
//			},
//		}
//
//		netAuth := &authpolicy.NetworkAuthentication{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1alpha1",
//				Kind:       "NetworkAuthentication",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      NetworkAuthenticationNameTemplate,
//				Namespace: intents.Namespace,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.NetworkAuthenticationSpec{
//				Networks: []*authpolicy.Network{
//					{
//						Cidr: "0.0.0.0/0",
//					},
//					{
//						Cidr: "::0",
//					},
//				},
//			},
//		}
//
//		mtlsAuth := &authpolicy.MeshTLSAuthentication{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1alpha1",
//				Kind:       "MeshTLSAuthentication",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, intents.Spec.Service.Name),
//				Namespace: intents.Namespace,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.MeshTLSAuthenticationSpec{
//				Identities: []string{"default.test-namespace.serviceaccount.identity.linkerd.cluster.local"},
//			},
//		}
//
//		server := &linkerdserver.Server{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1beta1",
//				Kind:       "Server",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "server-for-test-service-port-8000",
//				Namespace: ns,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: linkerdserver.ServerSpec{
//				PodSelector: &podSelector,
//				Port:        intstr.FromInt32(8000),
//			},
//		}
//
//		policy := &authpolicy.AuthorizationPolicy{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1alpha1",
//				Kind:       "AuthorizationPolicy",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "authpolicy-to-test-service-port-8000-from-client-service-that-calls-knng3afe",
//				Namespace: intents.Namespace,
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.AuthorizationPolicySpec{
//				TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
//					Group: "policy.linkerd.io",
//					Kind:  v1beta1.Kind("Server"),
//					Name:  v1beta1.ObjectName("server-for-test-service-port-8000"),
//				},
//				RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
//					{
//						Group: "policy.linkerd.io",
//						Kind:  v1beta1.Kind("MeshTLSAuthentication"),
//						Name:  "meshtls-for-client-service-that-calls",
//					},
//				},
//			},
//		}
//
//		oldPolicy := &authpolicy.AuthorizationPolicy{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1alpha1",
//				Kind:       "AuthorizationPolicy",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "very-ancient-policy-that-is-so-outdated",
//				Namespace: intents.Namespace,
//				UID:       "44445555-6666-7777-8888-9999aaaabbbb",
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: authpolicy.AuthorizationPolicySpec{
//				TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
//					Group: "policy.linkerd.io",
//					Kind:  v1beta1.Kind("Server"),
//					Name:  v1beta1.ObjectName("very-ancient-server"),
//				},
//				RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
//					{
//						Group: "policy.linkerd.io",
//						Kind:  v1beta1.Kind("MeshTLSAuthentication"),
//						Name:  "meshtls-for-client-service-that-calls",
//					},
//				},
//			},
//		}
//
//		oldServer := &linkerdserver.Server{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: "policy.linkerd.io/v1beta1",
//				Kind:       "Server",
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      "very-ancient-server",
//				Namespace: ns,
//				UID:       "aaabbbcc-dddd-eeee-ffff-111122223333",
//				Labels: map[string]string{
//					otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
//				},
//			},
//			Spec: linkerdserver.ServerSpec{
//				PodSelector: &podSelector,
//				Port:        intstr.FromInt32(420),
//			},
//		}
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(),
//			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity}).Do(func(_ context.Context, policies *authpolicy.AuthorizationPolicyList, _ ...client.ListOption) {
//			policies.Items = append(policies.Items, *oldPolicy)
//		}).Return(nil)
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(),
//			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity}).Do(func(_ context.Context, servers *linkerdserver.ServerList, _ ...client.ListOption) {
//			servers.Items = append(servers.Items, *oldServer)
//		}).Return(nil)
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(),
//			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity}).Return(nil)
//
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any(),
//			gomock.Any()).Do(func(_ context.Context, podList *v1.PodList, _ ...client.ListOption) {
//			podList.Items = append(podList.Items, pod)
//		})
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), netAuth).Return(nil)
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), mtlsAuth).Return(nil)
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), server)
//		s.Client.EXPECT().List(gomock.Any(), gomock.Any(), &client.ListOptions{Namespace: ns}).Return(nil)
//		s.Client.EXPECT().Create(gomock.Any(), policyWrapper{policy})
//
//		s.Client.EXPECT().Delete(gomock.Any(), oldPolicy).Return(nil)
//		s.Client.EXPECT().Delete(gomock.Any(), oldServer).Return(nil)
//
//		err := s.admin.Create(context.Background(), &intents, "default")
//		s.NoError(err)
//	}

func cleanEvents(events chan string) {
	// TODO: support expecting correct events in tests
	for len(events) > 0 {
		<-events
	}
}

func TestLinkerdManagerTestSuite(t *testing.T) {
	suite.Run(t, new(LinkerdManagerTestSuite))
}
