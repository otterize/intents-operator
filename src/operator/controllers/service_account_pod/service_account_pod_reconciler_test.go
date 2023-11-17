package service_account_pod

import (
	"context"
	"fmt"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	mock_client "github.com/otterize/credentials-operator/src/mocks/controller-runtime/client"
	mock_record "github.com/otterize/credentials-operator/src/mocks/eventrecorder"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"net/http"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type serviceAccountMatcher struct {
	Name      string
	Namespace string
	Labels    map[string]string
}

func (m *serviceAccountMatcher) String() string {
	return fmt.Sprintf("expected Name: %s Namespace: %s Labels: %s", m.Name, m.Namespace, m.Labels)
}

func (m *serviceAccountMatcher) Matches(x interface{}) bool {
	sa := x.(*v1.ServiceAccount)
	return sa.Name == m.Name && sa.Namespace == m.Namespace && reflect.DeepEqual(m.Labels, sa.Labels)
}

type PodServiceAccountEnsurerSuite struct {
	suite.Suite
	controller               *gomock.Controller
	client                   *mock_client.MockClient
	ServiceAccountReconciler *PodServiceAccountReconciler
	mockEventRecorder        *mock_record.MockEventRecorder
}

func (s *PodServiceAccountEnsurerSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())
	s.client = mock_client.NewMockClient(s.controller)

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	s.client.EXPECT().Scheme().Return(scheme).AnyTimes()
	s.mockEventRecorder = mock_record.NewMockEventRecorder(s.controller)
	s.ServiceAccountReconciler = NewPodServiceAccountReconciler(s.client, scheme, s.mockEventRecorder, nil)
}

func (s *PodServiceAccountEnsurerSuite) TestErrorWhenSADoesntExist() {
	serviceAccountName := "cool.name"
	annotations := map[string]string{metadata.CreateAWSRoleAnnotation: serviceAccountName}
	namespace := "namespace"
	s.client.EXPECT().Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: serviceAccountName, Namespace: namespace}), gomock.AssignableToTypeOf(&v1.ServiceAccount{})).
		Return(
			&k8serrors.StatusError{
				ErrStatus: metav1.Status{Status: metav1.StatusFailure, Code: http.StatusNotFound, Reason: metav1.StatusReasonNotFound},
			})

	s.mockEventRecorder.EXPECT().Eventf(gomock.Any(), gomock.Eq(v1.EventTypeWarning), gomock.Eq(ReasonGetServiceAccountFailed), gomock.Any(), gomock.Any())
	pod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "Pod", Namespace: namespace, Annotations: annotations}, Spec: v1.PodSpec{ServiceAccountName: serviceAccountName}}

	s.client.EXPECT().
		Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}), gomock.AssignableToTypeOf(&v1.Pod{})).
		Do(func(_, _ any, podPtr *v1.Pod, _ ...interface{}) {
			*podPtr = pod
		})

	res, err := s.ServiceAccountReconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}})
	s.Require().Error(err)
	s.Require().True(res.IsZero())

}

func (s *PodServiceAccountEnsurerSuite) TestUpdateWhenFound() {
	serviceAccountName := "cool.name"
	annotations := map[string]string{metadata.CreateAWSRoleAnnotation: serviceAccountName}
	namespace := "namespace"
	serviceAccount := v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name:      serviceAccountName,
		Namespace: namespace,
	}}
	s.client.EXPECT().Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: serviceAccountName, Namespace: namespace}), gomock.AssignableToTypeOf(&v1.ServiceAccount{})).
		Do(func(_, _ any, saPtr *v1.ServiceAccount, _ ...any) { *saPtr = serviceAccount })

	s.client.EXPECT().Patch(gomock.Any(), &serviceAccountMatcher{Name: serviceAccountName, Namespace: namespace, Labels: map[string]string{metadata.OtterizeServiceAccountLabel: serviceAccountName}}, gomock.AssignableToTypeOf(client.MergeFrom(&serviceAccount)))
	s.mockEventRecorder.EXPECT().Eventf(gomock.Any(), gomock.Eq(v1.EventTypeNormal), gomock.Eq(ReasonServiceAccountUpdated), gomock.Any(), gomock.Any())
	pod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "Pod", Namespace: namespace, Annotations: annotations}, Spec: v1.PodSpec{
		ServiceAccountName: serviceAccountName,
	}}

	s.client.EXPECT().
		Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}), gomock.AssignableToTypeOf(&v1.Pod{})).
		Do(func(_, _ any, podPtr *v1.Pod, _ ...interface{}) {
			*podPtr = pod
		})

	res, err := s.ServiceAccountReconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}})
	s.Require().NoError(err)
	s.Require().True(res.IsZero())

}

func (s *PodServiceAccountEnsurerSuite) TestDoNothingWhenNoAnnotation() {
	pod := v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "Pod", Namespace: "namespace"}}
	s.client.EXPECT().
		Get(gomock.Any(), gomock.Eq(types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}), gomock.AssignableToTypeOf(&v1.Pod{})).
		Do(func(_, _ any, podPtr *v1.Pod, _ ...interface{}) {
			*podPtr = pod
		})
	res, err := s.ServiceAccountReconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}})
	s.Require().NoError(err)
	s.Require().True(res.IsZero())
}

func TestPodServiceAccountEnsurerSuite(t *testing.T) {
	suite.Run(t, new(PodServiceAccountEnsurerSuite))
}
