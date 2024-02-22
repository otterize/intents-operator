package iam_pod_reconciler

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/aws_pod_reconciler/mocks"
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

type AWSPodReconcilerSuite struct {
	testbase.MocksSuiteBase
}

func (s *AWSPodReconcilerSuite) TestCallsAWSIntentsReconciler() {
	awsIntentsMockReconciler := mocks.NewMockReconciler(s.Controller)
	podReconciler := NewAWSPodReconciler(s.Client, s.Recorder, awsIntentsMockReconciler)
	namespacedName := types.NamespacedName{
		Name:      "pod",
		Namespace: "test",
	}

	intentsSpec := &otterizev1alpha3.IntentsSpec{
		Service: otterizev1alpha3.Service{Name: "service1"},
		Calls: []otterizev1alpha3.Intent{
			{
				Name: "arn:aws::s3:my-s3-bucket/*",
				Type: otterizev1alpha3.IntentTypeAWS,
				AWSActions: []string{
					"s3:*",
				},
			},
		},
	}
	clientIntents := otterizev1alpha3.ClientIntents{
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
		gomock.Eq(&otterizev1alpha3.ClientIntentsList{}),
		gomock.Any(), // FIXME client.MatchingFields{OtterizeClientNameIndexField: pod.Name},
		gomock.Any(), // FIXME &client.ListOptions{Namespace: namespacedName.Namespace}
	).DoAndReturn(
		func(arg0 context.Context, arg1 *otterizev1alpha3.ClientIntentsList, arg2 ...client.ListOption) error {
			clientIntentsList := otterizev1alpha3.ClientIntentsList{Items: []otterizev1alpha3.ClientIntents{clientIntents}}

			clientIntentsList.DeepCopyInto(arg1)
			return nil
		},
	)
	awsIntentsMockReconciler.EXPECT().Reconcile(gomock.Any(), reconcile.Request{NamespacedName: types.NamespacedName{
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

func TestAWSPodReconcilerSuite(t *testing.T) {
	suite.Run(t, new(AWSPodReconcilerSuite))
}
