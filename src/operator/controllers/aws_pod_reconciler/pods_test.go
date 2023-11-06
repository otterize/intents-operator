package aws_pod_reconciler

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/aws_pod_reconciler/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"testing"
)

type AWSPodReconcilerSuite struct {
	testbase.MocksSuiteBase
	PodReconciler *AWSPodReconciler
}

func (s *AWSPodReconcilerSuite) TestCallsAWSIntentsReconciler() {
	awsIntentsMockReconciler := mocks.NewMockReconciler(s.Controller)
	podReconciler := NewAWSPodReconciler(s.Client, s.Recorder, awsIntentsMockReconciler)
	namespacedName := types.NamespacedName{
		Name:      "pod",
		Namespace: "test",
	}

	s.Client.EXPECT().Get(gomock.Any(), namespacedName, &v1.Pod{})
	s.Client.EXPECT().List(
		gomock.Any(),
		&otterizev1alpha3.ClientIntentsList{},
		gomock.Any(),
		gomock.AssignableToTypeOf(&client.ListOptions{Namespace: namespacedName.Namespace}),
	)

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
