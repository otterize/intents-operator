package intents_reconcilers

import (
	"context"
	"github.com/golang/mock/gomock"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"testing"
	"time"
)

const (
	testNamespace = "test-namespace"
)

type ClientPatch struct {
	client.Patch
}

func (p ClientPatch) Matches(x interface{}) bool {
	patch := x.(client.Patch)
	actualData, err := patch.Data(nil)
	if err != nil {
		return false
	}

	expectedData, err := p.Data(nil)
	if err != nil {
		return false
	}

	return string(actualData) == string(expectedData) && patch.Type() == p.Type()
}

func (p ClientPatch) String() string {
	data, err := p.Data(nil)
	if err != nil {
		return "format error"
	}
	return string(data)
}

func MatchPatch(patch client.Patch) gomock.Matcher {
	return ClientPatch{patch}
}

type PodLabelReconcilerTestSuite struct {
	suite.Suite
	Reconciler *PodLabelReconciler
	client     *mocks.MockClient
}

func (s *PodLabelReconcilerTestSuite) SetupTest() {
	controller := gomock.NewController(s.T())
	s.client = mocks.NewMockClient(controller)
	s.Reconciler = NewPodLabelReconciler(s.client, nil)
}

func (s *PodLabelReconcilerTestSuite) TestClientAccessLabelAdded() {
	deploymentName := "test-deployment-name"
	serviceName := "test-client"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      deploymentName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := "test-server"
	intentsSpec := otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			intents.Spec = &intentsSpec
			controllerutil.AddFinalizer(intents, PodLabelFinalizerName)
			return nil
		})

	var intents otterizev1alpha2.ClientIntents
	intents.Spec = &intentsSpec

	listOption := &client.ListOptions{Namespace: testNamespace}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"intents.otterize.com/server": "test-client--2436df",
	})

	labelMatcher := client.MatchingLabelsSelector{Selector: labelSelector}
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-pod",
			Labels: make(map[string]string),
		},
	}

	s.client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Eq(listOption), gomock.Eq(labelMatcher)).DoAndReturn(
		func(ctx context.Context, pds *v1.PodList, opts ...client.ListOption) error {
			pds.Items = append(pds.Items, pod)
			return nil
		})

	updatedPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Labels: map[string]string{
				"intents.otterize.com/access-test-server-test-namespace-8ddecb": "true",
				"intents.otterize.com/client":                                   "true",
			},
		},
		Spec: v1.PodSpec{},
	}

	s.client.EXPECT().Patch(gomock.Any(), gomock.Eq(&updatedPod), gomock.Any()).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func (s *PodLabelReconcilerTestSuite) TestClientAccessLabelAddedTruncatedNameAndNamespace() {
	deploymentName := "test-deployment-name"
	serviceName := "test-client-with-a-very-long-name-more-than-20-characters"
	longNamespace := "test-namespace-with-a-very-long-name-more-than-20-characters"

	namespacedName := types.NamespacedName{
		Namespace: longNamespace,
		Name:      deploymentName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := "test-server"
	intentsSpec := otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			intents.Spec = &intentsSpec
			controllerutil.AddFinalizer(intents, PodLabelFinalizerName)
			return nil
		})

	var intents otterizev1alpha2.ClientIntents
	intents.Spec = &intentsSpec

	listOption := &client.ListOptions{Namespace: longNamespace}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"intents.otterize.com/server": "test-client-with-a-v--ef05bc",
	})

	labelMatcher := client.MatchingLabelsSelector{Selector: labelSelector}
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-pod",
			Labels: make(map[string]string),
		},
	}

	s.client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Eq(listOption), gomock.Eq(labelMatcher)).DoAndReturn(
		func(ctx context.Context, pds *v1.PodList, opts ...client.ListOption) error {
			pds.Items = append(pds.Items, pod)
			return nil
		})

	updatedPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Labels: map[string]string{
				"intents.otterize.com/access-test-server-test-namespace-with--a1ac14": "true",
				"intents.otterize.com/client":                                         "true",
			},
		},
		Spec: v1.PodSpec{},
	}

	s.client.EXPECT().Patch(gomock.Any(), gomock.Eq(&updatedPod), gomock.Any()).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func (s *PodLabelReconcilerTestSuite) TestClientAccessLabelRemoved() {
	s.testClientAccessLabelRemovedWithParams(map[string]string{"a": "b"})
}

func (s *PodLabelReconcilerTestSuite) TestClientAccessLabelRemovedNoPodAnnotations() {
	s.testClientAccessLabelRemovedWithParams(nil)
}

func (s *PodLabelReconcilerTestSuite) testClientAccessLabelRemovedWithParams(podAnnotations map[string]string) {
	deploymentName := "test-deployment-name"
	serviceName := "test-client"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      deploymentName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := "test-server"
	intentsSpec := otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	emptyIntents := &otterizev1alpha2.ClientIntents{}

	var deletedIntents otterizev1alpha2.ClientIntents
	deletedIntents.Spec = &intentsSpec
	deletedIntents.Namespace = testNamespace
	deletedIntents.SetDeletionTimestamp(&metav1.Time{Time: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
	controllerutil.AddFinalizer(&deletedIntents, PodLabelFinalizerName)

	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			*intents = deletedIntents
			return nil
		})

	// Now the reconciler should handle the deletion of the client intents

	deletedIntentsWithoutFinalizer := deletedIntents.DeepCopy()
	deletedIntentsWithoutFinalizer.SetFinalizers([]string{})
	s.client.EXPECT().Update(gomock.Any(), gomock.Eq(deletedIntentsWithoutFinalizer)).Return(nil)
	listOption := &client.ListOptions{Namespace: testNamespace}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"intents.otterize.com/server": "test-client-test-namespace-537e87",
	})

	labelMatcher := client.MatchingLabelsSelector{Selector: labelSelector}
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Labels: map[string]string{
				"intents.otterize.com/access-test-server-test-namespace-8ddecb": "true",
				otterizev1alpha2.OtterizeClientLabelKey:                         "true",
			},
			Annotations: podAnnotations,
		},
		Spec: v1.PodSpec{},
	}

	s.client.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&v1.PodList{}), gomock.Eq(listOption), gomock.Eq(labelMatcher)).DoAndReturn(
		func(ctx context.Context, pds *v1.PodList, opts ...client.ListOption) error {
			pds.Items = append(pds.Items, pod)
			return nil
		})

	if podAnnotations == nil {
		podAnnotations = make(map[string]string)
	}

	podAnnotations[otterizev1alpha2.AllIntentsRemovedAnnotation] = "true"
	updatedPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Labels: map[string]string{
				otterizev1alpha2.OtterizeClientLabelKey: "true",
			},
			Annotations: podAnnotations,
		},
		Spec: v1.PodSpec{},
	}

	s.client.EXPECT().Patch(gomock.Any(), gomock.Eq(&updatedPod), MatchPatch(client.MergeFrom(&pod))).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func (s *PodLabelReconcilerTestSuite) TestAccessLabelChangedOnIntentsEdit() {
	deploymentName := "test-deployment-name"
	serviceName := "test-client"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      deploymentName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := "test-server"
	intentsSpec := otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			intents.Spec = &intentsSpec
			controllerutil.AddFinalizer(intents, PodLabelFinalizerName)
			return nil
		})

	var intents otterizev1alpha2.ClientIntents
	intents.Spec = &intentsSpec

	listOption := &client.ListOptions{Namespace: testNamespace}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"intents.otterize.com/server": "test-client--2436df",
	})

	labelMatcher := client.MatchingLabelsSelector{Selector: labelSelector}
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-pod",
			Labels: make(map[string]string),
		},
	}

	s.client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Eq(listOption), gomock.Eq(labelMatcher)).DoAndReturn(
		func(ctx context.Context, pds *v1.PodList, opts ...client.ListOption) error {
			pds.Items = append(pds.Items, pod)
			return nil
		})

	updatedPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Labels: map[string]string{
				"intents.otterize.com/access-test-server-test-namespace-8ddecb": "true",
				"intents.otterize.com/client":                                   "true",
			},
		},
		Spec: v1.PodSpec{},
	}

	s.client.EXPECT().Patch(gomock.Any(), gomock.Eq(&updatedPod), gomock.Any()).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)

	// Now all the way through again, but with a different server name

	intentsSpec.Calls[0].Name = "test-server-2"

	emptyIntents = &otterizev1alpha2.ClientIntents{}
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			intents.Spec = &intentsSpec
			controllerutil.AddFinalizer(intents, PodLabelFinalizerName)
			return nil
		})

	s.client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Eq(listOption), gomock.Eq(labelMatcher)).DoAndReturn(
		func(ctx context.Context, pds *v1.PodList, opts ...client.ListOption) error {
			pds.Items = append(pds.Items, pod)
			return nil
		})

	updatedPod = v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Labels: map[string]string{
				"intents.otterize.com/access-test-server-2-test-namespace-e4423b": "true",
				"intents.otterize.com/client":                                     "true",
			},
		},
		Spec: v1.PodSpec{},
	}

	s.client.EXPECT().Patch(gomock.Any(), gomock.Eq(&updatedPod), gomock.Any()).Return(nil)

	res, err = s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)

}

func (s *PodLabelReconcilerTestSuite) TestPodLabelFinalizerAdded() {
	deploymentName := "test-deployment-name"
	serviceName := "test-client"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      deploymentName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := "test-server"
	intentsSpec := otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	emptyIntents := &otterizev1alpha2.ClientIntents{}
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			intents.Spec = &intentsSpec
			return nil
		})

	var intents otterizev1alpha2.ClientIntents
	intents.Spec = &intentsSpec
	controllerutil.AddFinalizer(&intents, PodLabelFinalizerName)
	s.client.EXPECT().Update(gomock.Any(), gomock.Eq(&intents)).Return(nil)

	s.client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func (s *PodLabelReconcilerTestSuite) TestPodLabelFinalizerRemoved() {
	deploymentName := "test-deployment-name"
	serviceName := "test-client"

	namespacedName := types.NamespacedName{
		Namespace: testNamespace,
		Name:      deploymentName,
	}
	req := ctrl.Request{
		NamespacedName: namespacedName,
	}

	serverName := "test-server"
	intentsSpec := otterizev1alpha2.IntentsSpec{
		Service: otterizev1alpha2.Service{Name: serviceName},
		Calls: []otterizev1alpha2.Intent{
			{
				Name: serverName,
			},
		},
	}

	emptyIntents := &otterizev1alpha2.ClientIntents{}
	deletionTimestamp := &metav1.Time{Time: time.Now()}
	s.client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.Eq(emptyIntents)).DoAndReturn(
		func(ctx context.Context, name types.NamespacedName, intents *otterizev1alpha2.ClientIntents, options ...client.ListOption) error {
			intents.Spec = &intentsSpec
			controllerutil.AddFinalizer(intents, PodLabelFinalizerName)
			intents.DeletionTimestamp = deletionTimestamp
			return nil
		})

	intentsWithoutFinalizer := otterizev1alpha2.ClientIntents{}
	intentsWithoutFinalizer.Spec = &intentsSpec
	intentsWithoutFinalizer.DeletionTimestamp = deletionTimestamp
	controllerutil.AddFinalizer(&intentsWithoutFinalizer, PodLabelFinalizerName)
	controllerutil.RemoveFinalizer(&intentsWithoutFinalizer, PodLabelFinalizerName)

	s.client.EXPECT().Update(gomock.Any(), gomock.Eq(&intentsWithoutFinalizer)).Return(nil)
	s.client.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	res, err := s.Reconciler.Reconcile(context.Background(), req)
	s.NoError(err)
	s.Equal(ctrl.Result{}, res)
}

func TestPodLabelReconcilerTestSuite(t *testing.T) {
	suite.Run(t, new(PodLabelReconcilerTestSuite))
}
