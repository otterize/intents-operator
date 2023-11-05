package aws_pod_reconciler

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/controllers/aws_pod_reconciler/mocks"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/types"
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
	res, err := podReconciler.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "pod"}})
	s.Require().Empty(res)
	s.Require().NoError(err)
}

func TestAWSPodReconcilerSuite(t *testing.T) {
	suite.Run(t, new(AWSPodReconcilerSuite))
}
