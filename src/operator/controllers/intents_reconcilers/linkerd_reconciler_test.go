package intents_reconcilers

import (
	"context"

	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/testbase"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type LinkerdReconcilerTestSuite struct {
	testbase.MocksSuiteBase
	admin *LinkerdReconciler
}

func (s *LinkerdReconcilerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()
	s.admin = NewLinkerdReconciler(s.Client, &runtime.Scheme{}, []string{}, true, true)
}

func (s *LinkerdReconcilerTestSuite) TearDownTest() {
	s.admin = nil
	s.MocksSuiteBase.TearDownTest()
}

func (s *LinkerdReconcilerTestSuite) TestAnything() {
	_ = &v1alpha3.ClientIntents{
		ObjectMeta: v1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: &v1alpha3.IntentsSpec{
			Service: v1alpha3.Service{
				Name: clientName,
			},
			Calls: []v1alpha3.Intent{
				{
					Name: "test",
				},
			},
		},
	}

	s.admin.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "test",
		},
	})
}
