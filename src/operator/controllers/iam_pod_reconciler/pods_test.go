package iam_pod_reconciler

import (
	"context"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"github.com/otterize/intents-operator/src/operator/controllers/iam_pod_reconciler/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

type IAMPodReconcilerSuite struct {
	testbase.MocksSuiteBase
}

func (s *IAMPodReconcilerSuite) TestCallsIAMIntentsReconciler() {
	iamIntentsMockReconciler := mocks.NewMockReconciler(s.Controller)
	podReconciler := NewIAMPodReconciler(s.Client, s.Recorder, iamIntentsMockReconciler)
	namespacedName := types.NamespacedName{
		Name:      "pod",
		Namespace: "test",
	}

	intentsSpec := &otterizev2.IntentsSpec{
		Workload: otterizev2.Workload{Name: "service1"},
		Targets: []otterizev2.Target{
			{
				AWS: &otterizev2.AWSTarget{
					ARN: "arn:aws::s3:my-s3-bucket/*",
					Actions: []string{
						"s3:*",
					},
				},
			},
		},
	}
	clientIntents := otterizev2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "client-intents",
			Namespace: "test",
		},
		Spec: intentsSpec,
	}

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "test",
			Annotations: map[string]string{
				"intents.otterize.com/service-name": "client-A",
			},
		},
	}
	s.Client.EXPECT().Get(gomock.Any(), namespacedName, &corev1.Pod{}).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *corev1.Pod, arg3 ...client.GetOption) error {
			pod.DeepCopyInto(arg2)
			return nil
		},
	)
	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.Eq(&otterizev2.ClientIntentsList{}),
		gomock.Any(), // FIXME client.MatchingFields{OtterizeClientNameIndexField: pod.Name},
		gomock.Any(), // FIXME &client.ListOptions{Namespace: namespacedName.Namespace}
	).DoAndReturn(
		func(arg0 context.Context, arg1 *otterizev2.ClientIntentsList, arg2 ...client.ListOption) error {
			clientIntentsList := otterizev2.ClientIntentsList{Items: []otterizev2.ClientIntents{clientIntents}}

			clientIntentsList.DeepCopyInto(arg1)
			return nil
		},
	)
	iamIntentsMockReconciler.EXPECT().Reconcile(gomock.Any(), reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      clientIntents.Name,
		Namespace: clientIntents.Namespace,
	}})

	res, err := podReconciler.Reconcile(
		context.Background(),
		reconcile.Request{
			NamespacedName: namespacedName,
		},
	)

	s.Require().Empty(res)
	s.Require().NoError(err)
}

func TestIAMPodReconcilerSuite(t *testing.T) {
	suite.Run(t, new(IAMPodReconcilerSuite))
}
