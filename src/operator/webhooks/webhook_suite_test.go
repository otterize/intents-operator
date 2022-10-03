// /*
// Copyright 2022.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// */
package webhooks

import (
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/kubernetes"
	"path/filepath"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

type ValidationWebhookTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
}

func (s *ValidationWebhookTestSuite) SetupSuite() {
	s.TestEnv = &envtest.Environment{}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "config", "crd", "bases")}
	s.TestEnv.WebhookInstallOptions = envtest.WebhookInstallOptions{
		Paths:            []string{filepath.Join("..", "config", "webhook")},
		LocalServingPort: 9443,
		LocalServingHost: "localhost",
	}

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)

	err = otterizev1alpha1.AddToScheme(s.TestEnv.Scheme)
	s.Require().NoError(err)

}

func (s *ValidationWebhookTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	intentsValidator := NewIntentsValidator(s.Mgr.GetClient())
	s.Require().NoError(intentsValidator.SetupWebhookWithManager(s.Mgr))
	s.Mgr.GetWebhookServer().CertDir = s.TestEnv.WebhookInstallOptions.LocalServingCertDir
	s.Mgr.GetWebhookServer().Host = s.TestEnv.WebhookInstallOptions.LocalServingHost
}

func (s *ValidationWebhookTestSuite) TestNoDuplicateClientsAllowed() {
	_, err := s.AddIntents("intents", "someclient", []otterizev1alpha1.Intent{})
	s.Require().NoError(err)

	_, err = s.AddIntents("intents2", "someclient", []otterizev1alpha1.Intent{})
	s.Require().ErrorContains(err, "Intents for client someclient already exist in resource")
}

func (s *ValidationWebhookTestSuite) TestNoTopicsForHTTPIntents() {
	_, err := s.AddIntents("intents", "someclient", []otterizev1alpha1.Intent{
		{
			Type: otterizev1alpha1.IntentTypeHTTP,
			Topics: []otterizev1alpha1.KafkaTopic{{
				Name:      "sometopic",
				Operation: otterizev1alpha1.KafkaOperationConsume,
			}},
		},
	})
	s.Require().ErrorContains(err, "type 'HTTP' cannot contain kafka topics")
}

func TestValidationWebhookTestSuite(t *testing.T) {
	suite.Run(t, new(ValidationWebhookTestSuite))
}
