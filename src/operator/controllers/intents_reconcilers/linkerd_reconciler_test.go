package intents_reconcilers

import (
	"context"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	v12 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"testing"

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

	// This object will be returned to the reconciler's Get call when it calls for a CRD named "servers.policy.linkerd.io"
	crd := &v12.CustomResourceDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name: "servers.policy.linkerd.io",
		},
	}

	// The matchers here make it check for a CRD called "servers.policy.linkerd.io"
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: "servers.policy.linkerd.io"}, gomock.AssignableToTypeOf(crd)).Do(
		func(ctx context.Context, key types.NamespacedName, obj *v12.CustomResourceDefinition, opts ...v1.GetOptions) error {
			// Copy the struct into the target pointer struct
			*obj = *crd
			return nil
		})

	res, err := s.admin.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test",
			Namespace: "test",
		},
	})
	s.Require().NoError(err)
	s.Require().Empty(res)
}

func TestLinkerdReconcilerSuite(t *testing.T) {
	suite.Run(t, new(LinkerdReconcilerTestSuite))
}
