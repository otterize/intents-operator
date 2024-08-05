package controllers

import (
	"context"
	"encoding/base64"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/operator/otterizecrds"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/otterize/intents-operator/src/shared/testbase"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

const TestCRD = "clientintents.k8s.otterize.com"
const TestControllerNamespace = "test-namespace"

type CustomResourceDefinitionsTestSuite struct {
	testbase.MocksSuiteBase
	Cert          []byte
	crdReconciler *CustomResourceDefinitionsReconciler
}

func (s *CustomResourceDefinitionsTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()

	base64Cert := base64.StdEncoding.EncodeToString([]byte("test"))
	s.Cert = []byte(base64Cert)

	s.crdReconciler = NewCustomResourceDefinitionsReconciler(
		s.Client,
		scheme.Scheme,
		s.Cert,
		TestControllerNamespace,
	)
}

func (s *CustomResourceDefinitionsTestSuite) TearDownTest() {
	s.MocksSuiteBase.TearDownTest()
}

func (s *CustomResourceDefinitionsTestSuite) TestAssigningCABundle() {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Name: TestCRD},
	}

	// Create a test webhook configuration object
	crd, err := otterizecrds.GetCRDDefinitionByName(TestCRD)
	s.Require().NoError(err)

	copyCRD := crd.DeepCopy()
	copyCRD.ResourceVersion = "1"

	updatedCRD, err := otterizecrds.GetCRDDefinitionByName(TestCRD)
	s.Require().NoError(err)

	updatedCRD.Spec.Conversion.Webhook.ClientConfig.CABundle = s.Cert
	updatedCRD.Spec.Conversion.Webhook.ClientConfig.Service.Namespace = TestControllerNamespace
	updatedCRD.ResourceVersion = "1"

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(crd)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *apiextensionsv1.CustomResourceDefinition, arg3 ...client.GetOption) error {
			*arg2 = *copyCRD
			return nil
		},
	)

	matcher := intents_reconcilers.MatchPatch(client.MergeFromWithOptions(copyCRD, client.MergeFromWithOptimisticLock{}))
	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(updatedCRD), matcher).Return(nil)

	// Call the reconcile function
	_, err = s.crdReconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
}

func TestCustomResourceDefinitionsTestSuite(t *testing.T) {
	suite.Run(t, new(CustomResourceDefinitionsTestSuite))
}
