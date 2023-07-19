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
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"testing"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

type ValidationWebhookTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
}

func (s *ValidationWebhookTestSuite) SetupSuite() {
	s.TestEnv = &envtest.Environment{}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "config", "crd")}
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

	utilruntime.Must(apiextensionsv1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(istiosecurityscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha2.AddToScheme(s.TestEnv.Scheme))

}

func (s *ValidationWebhookTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	intentsValidator := NewIntentsValidator(s.Mgr.GetClient())
	s.Require().NoError(intentsValidator.SetupWebhookWithManager(s.Mgr))
	s.Mgr.GetWebhookServer().CertDir = s.TestEnv.WebhookInstallOptions.LocalServingCertDir
	s.Mgr.GetWebhookServer().Host = s.TestEnv.WebhookInstallOptions.LocalServingHost
}

func (s *ValidationWebhookTestSuite) TestNoDuplicateClientsAllowed() {
	_, err := s.AddIntents("intents", "someclient", []otterizev1alpha2.Intent{})
	s.Require().NoError(err)

	_, err = s.AddIntents("intents2", "someclient", []otterizev1alpha2.Intent{})
	s.Require().ErrorContains(err, "Intents for client someclient already exist in resource")
}

func (s *ValidationWebhookTestSuite) TestNoTopicsForHTTPIntents() {
	_, err := s.AddIntents("intents", "someclient", []otterizev1alpha2.Intent{
		{
			Type: otterizev1alpha2.IntentTypeHTTP,
			Topics: []otterizev1alpha2.KafkaTopic{{
				Name:       "sometopic",
				Operations: []otterizev1alpha2.KafkaOperation{otterizev1alpha2.KafkaOperationConsume},
			}},
		},
	})
	expectedErr := fmt.Sprintf("type %s cannot contain kafka topics", otterizev1alpha2.IntentTypeHTTP)
	s.Require().ErrorContains(err, expectedErr)
}

func (s *ValidationWebhookTestSuite) TestNoTopicsForHTTPIntentsAfterUpdate() {
	_, err := s.AddIntents("intents", "someclient", []otterizev1alpha2.Intent{
		{
			Type: otterizev1alpha2.IntentTypeKafka,
			Topics: []otterizev1alpha2.KafkaTopic{{
				Name:       "sometopic",
				Operations: []otterizev1alpha2.KafkaOperation{otterizev1alpha2.KafkaOperationConsume},
			}},
		},
	})
	expectedErr := fmt.Sprintf("type %s cannot contain kafka topics", otterizev1alpha2.IntentTypeHTTP)
	s.Require().NoError(err)

	err = s.UpdateIntents("intents", []otterizev1alpha2.Intent{
		{
			Type: otterizev1alpha2.IntentTypeHTTP,
			Topics: []otterizev1alpha2.KafkaTopic{{
				Name:       "sometopic",
				Operations: []otterizev1alpha2.KafkaOperation{otterizev1alpha2.KafkaOperationConsume},
			}},
		},
	})
	s.Require().ErrorContains(err, expectedErr)
}

func (s *ValidationWebhookTestSuite) TestValidateProtectedServices() {
	fakeValidator := NewProtectedServicesValidator(nil)

	protectedServices := otterizev1alpha2.ProtectedServices{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-services",
			Namespace: "test-namespace",
		},
		Spec: otterizev1alpha2.ProtectedServicesSpec{
			ProtectedServices: []otterizev1alpha2.ProtectedService{
				{
					Name: "my-service",
				},
				{
					Name: "myservice2",
				},
				{
					Name: "my_service3",
				},
			},
		},
	}
	err := fakeValidator.validateSpec(&protectedServices)
	s.Require().True(err == nil)
}

func (s *ValidationWebhookTestSuite) TestValidateProtectedServicesFailIfDotFound() {
	fakeValidator := NewProtectedServicesValidator(nil)

	protectedServices := otterizev1alpha2.ProtectedServices{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-services",
			Namespace: "test-namespace",
		},
		Spec: otterizev1alpha2.ProtectedServicesSpec{
			ProtectedServices: []otterizev1alpha2.ProtectedService{
				{
					Name: "my-service.test-namespace",
				},
			},
		},
	}
	err := fakeValidator.validateSpec(&protectedServices)
	s.Require().Error(err)
}

func (s *ValidationWebhookTestSuite) TestValidateProtectedServicesFailIfUppercase() {
	fakeValidator := NewProtectedServicesValidator(nil)

	protectedServices := otterizev1alpha2.ProtectedServices{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-services",
			Namespace: "test-namespace",
		},
		Spec: otterizev1alpha2.ProtectedServicesSpec{
			ProtectedServices: []otterizev1alpha2.ProtectedService{
				{
					Name: "MyService",
				},
			},
		},
	}
	err := fakeValidator.validateSpec(&protectedServices)
	s.Require().Error(err)
}

func TestValidationWebhookTestSuite(t *testing.T) {
	suite.Run(t, new(ValidationWebhookTestSuite))
}
