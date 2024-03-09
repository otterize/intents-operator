package mutatingwebhookconfiguration

import (
	"context"
	"encoding/base64"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers"
	"github.com/otterize/intents-operator/src/shared/filters"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/otterize/intents-operator/src/shared/testbase"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

const TestWebhook = "test-webhook"
const TestWebhookConfig = "test-webhook-config"

type MutatingWebhookControllerTestSuite struct {
	testbase.MocksSuiteBase
	Cert                      []byte
	mutatingWebhookReconciler *MutatingWebhookConfigsReconciler
}

func (s *MutatingWebhookControllerTestSuite) SetupTest() {
	s.MocksSuiteBase.SetupTest()

	base64Cert := base64.StdEncoding.EncodeToString([]byte("test"))
	s.Cert = []byte(base64Cert)

	s.mutatingWebhookReconciler = NewMutatingWebhookConfigsReconciler(
		s.Client,
		scheme.Scheme,
		s.Cert,
		filters.CredentialsOperatorLabelPredicate(),
	)
}

func (s *MutatingWebhookControllerTestSuite) TearDownTest() {
	s.MocksSuiteBase.TearDownTest()
}

func (s *MutatingWebhookControllerTestSuite) TestAssigningCABundle() {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{Name: TestWebhookConfig},
	}

	// Create a test webhook configuration object
	webhookConfig := admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: TestWebhookConfig,
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: TestWebhook,
			},
		},
	}

	updatedWebhookConfig := admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: TestWebhookConfig,
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: TestWebhook,
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: s.Cert,
				},
			},
		},
	}

	s.Client.EXPECT().Get(gomock.Any(), req.NamespacedName, gomock.AssignableToTypeOf(&webhookConfig)).DoAndReturn(
		func(arg0 context.Context, arg1 types.NamespacedName, arg2 *admissionregistrationv1.MutatingWebhookConfiguration, arg3 ...client.GetOption) error {
			*arg2 = webhookConfig
			return nil
		},
	)

	matcher := intents_reconcilers.MatchPatch(client.MergeFrom(&webhookConfig))
	s.Client.EXPECT().Patch(gomock.Any(), gomock.Eq(&updatedWebhookConfig), matcher).Return(nil)

	// Call the reconcile function
	_, err := s.mutatingWebhookReconciler.Reconcile(context.Background(), req)
	s.Require().NoError(err)
}

func TestMutatingWebhookControllerTestSuite(t *testing.T) {
	suite.Run(t, new(MutatingWebhookControllerTestSuite))
}
