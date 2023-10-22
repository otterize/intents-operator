package external_traffic

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	otterizecloudmocks "github.com/otterize/intents-operator/src/shared/otterizecloud/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

const (
	testNamespace = "test-namespace"
)

type NetworkPolicyReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	reconciler  *NetworkPolicyUploaderReconciler
	cloudClient *otterizecloudmocks.MockCloudClient
}

func (s *NetworkPolicyReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()

	controller := gomock.NewController(s.T())
	s.cloudClient = otterizecloudmocks.NewMockCloudClient(controller)
	s.reconciler = NewNetworkPolicyUploaderReconciler(
		s.Client,
		&runtime.Scheme{},
		s.cloudClient,
	)

	s.reconciler.Recorder = s.Recorder
}

func (s *NetworkPolicyReconcilerTestSuite) TearDownTest() {
	s.cloudClient = nil
	s.reconciler = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *NetworkPolicyReconcilerTestSuite) TestUploadNetworkPolicy() {
	accessNetworkPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-access-to-client-A",
			Namespace: testNamespace,
			Labels: map[string]string{
				v1alpha2.OtterizeNetworkPolicyExternalTraffic: "client-A",
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      v1alpha2.OtterizeServerLabelKey,
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				{},
			},
		},
	}

	emptyNetworkPolicy := v1.NetworkPolicy{}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      accessNetworkPolicy.Name,
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(&emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, namespacedName types.NamespacedName, networkPolicy *v1.NetworkPolicy, _ ...client.GetOption) error {
			accessNetworkPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	podList := &corev1.PodList{}
	selector, err := metav1.LabelSelectorAsSelector(&accessNetworkPolicy.Spec.PodSelector)
	s.Require().NoError(err)

	s.Client.EXPECT().List(gomock.Any(),
		gomock.Eq(podList),
		&client.MatchingLabelsSelector{Selector: selector},
		&client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, podList *corev1.PodList, listOptions ...client.ListOption) error {
			podList.Items = []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod-1",
						Namespace: testNamespace,
						Annotations: map[string]string{
							"intents.otterize.com/service-name": "client-A",
						},
					},
				},
			}
			return nil
		})

	uploadedNetworkPolicy := graphqlclient.NetworkPolicyInput{
		Namespace:                    testNamespace,
		Name:                         accessNetworkPolicy.Name,
		ServerName:                   "client-A",
		ExternalNetworkTrafficPolicy: true,
	}

	s.cloudClient.EXPECT().ReportNetworkPolicies(gomock.Any(), testNamespace, []graphqlclient.NetworkPolicyInput{uploadedNetworkPolicy}).Return(nil)

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func (s *NetworkPolicyReconcilerTestSuite) TestNoUploadIfNoPods() {
	accessNetworkPolicy := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "external-access-to-client-A",
			Namespace: testNamespace,
			Labels: map[string]string{
				v1alpha2.OtterizeNetworkPolicyExternalTraffic: "client-A",
			},
		},
		Spec: v1.NetworkPolicySpec{
			PolicyTypes: []v1.PolicyType{v1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      v1alpha2.OtterizeServerLabelKey,
						Operator: metav1.LabelSelectorOpExists,
					},
				},
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				{},
			},
		},
	}

	emptyNetworkPolicy := v1.NetworkPolicy{}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: testNamespace,
			Name:      accessNetworkPolicy.Name,
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(&emptyNetworkPolicy)).DoAndReturn(
		func(ctx context.Context, namespacedName types.NamespacedName, networkPolicy *v1.NetworkPolicy, _ ...client.GetOption) error {
			accessNetworkPolicy.DeepCopyInto(networkPolicy)
			return nil
		})

	podList := &corev1.PodList{}
	selector, err := metav1.LabelSelectorAsSelector(&accessNetworkPolicy.Spec.PodSelector)
	s.Require().NoError(err)

	s.Client.EXPECT().List(gomock.Any(),
		gomock.Eq(podList),
		&client.MatchingLabelsSelector{Selector: selector},
		&client.ListOptions{Namespace: testNamespace}).DoAndReturn(
		func(ctx context.Context, podList *corev1.PodList, listOptions ...client.ListOption) error {
			podList.Items = []corev1.Pod{}
			return nil
		})

	// Expect no calls to cloud client

	res, err := s.reconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Equal(reconcile.Result{RequeueAfter: retryGettingPodsForPolicy}, res)
}

func TestNetworkPolicyReconcilerSuite(t *testing.T) {
	suite.Run(t, new(NetworkPolicyReconcilerTestSuite))
}
