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
	"context"
	"errors"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/stretchr/testify/suite"
	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"net"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"strconv"
	"testing"
	"time"
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
	utilruntime.Must(otterizev1alpha3.AddToScheme(s.TestEnv.Scheme))

}

func (s *ValidationWebhookTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	intentsValidator := NewIntentsValidator(s.Mgr.GetClient())
	s.Require().NoError(intentsValidator.SetupWebhookWithManager(s.Mgr))
	s.Mgr.GetWebhookServer().CertDir = s.TestEnv.WebhookInstallOptions.LocalServingCertDir
	s.Mgr.GetWebhookServer().Host = s.TestEnv.WebhookInstallOptions.LocalServingHost
	s.Mgr.GetWebhookServer().Port = s.TestEnv.WebhookInstallOptions.LocalServingPort

}

func (s *ValidationWebhookTestSuite) BeforeTest(suiteName, testName string) {
	s.ControllerManagerTestSuiteBase.BeforeTest(suiteName, testName)
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for err := errors.New("dummy"); err != nil; _, err = net.Dial("tcp", net.JoinHostPort(s.TestEnv.WebhookInstallOptions.LocalServingHost, strconv.Itoa(s.TestEnv.WebhookInstallOptions.LocalServingPort))) {
		select {
		case <-ctxTimeout.Done():
			s.Require().FailNow("timeout waiting to connect to webhook server")
		case <-time.After(200 * time.Millisecond):

		}
	}
}

func (s *ValidationWebhookTestSuite) TestNoDuplicateClientsAllowed() {
	_, err := s.AddIntentsV1alpha2("intents", "someclient", []otterizev1alpha2.Intent{})
	s.Require().NoError(err)

	_, err = s.AddIntentsV1alpha2("intents2", "someclient", []otterizev1alpha2.Intent{})
	s.Require().ErrorContains(err, "Intents for client someclient already exist in resource")
}

func (s *ValidationWebhookTestSuite) TestNoTopicsForHTTPIntents() {
	_, err := s.AddIntentsV1alpha2("intents", "someclient", []otterizev1alpha2.Intent{
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
	_, err := s.AddIntentsV1alpha2("intents", "someclient", []otterizev1alpha2.Intent{
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

	err = s.UpdateIntentsV1alpha2("intents", []otterizev1alpha2.Intent{
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
	fakeValidator := NewProtectedServiceValidator(nil)

	protectedServices := []otterizev1alpha2.ProtectedService{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-services",
			Namespace: "test-namespace",
		},
		Spec: otterizev1alpha2.ProtectedServiceSpec{
			Name: "my-service",
		},
	},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "protected-services",
				Namespace: "test-namespace",
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: "myservice2",
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "protected-services",
				Namespace: "test-namespace",
			},
			Spec: otterizev1alpha2.ProtectedServiceSpec{
				Name: "my_service3",
			},
		},
	}
	for _, service := range protectedServices {
		s.Require().True(fakeValidator.validateSpec(&service) == nil)
	}
}

func (s *ValidationWebhookTestSuite) TestValidateProtectedServicesFailIfDotFound() {
	fakeValidator := NewProtectedServiceValidator(nil)

	protectedServices := otterizev1alpha2.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-services",
			Namespace: "test-namespace",
		},
		Spec: otterizev1alpha2.ProtectedServiceSpec{
			Name: "my-service.test-namespace",
		},
	}
	err := fakeValidator.validateSpec(&protectedServices)
	s.Require().Error(err)
}

func (s *ValidationWebhookTestSuite) TestValidateProtectedServicesFailIfSameName() {
	fakeValidator := NewProtectedServiceValidator(nil)

	protectedServiceList := otterizev1alpha2.ProtectedServiceList{Items: []otterizev1alpha2.ProtectedService{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-services",
			Namespace: "test-namespace",
		},
		Spec: otterizev1alpha2.ProtectedServiceSpec{
			Name: "my-service.test-namespace",
		},
	}}}

	protectedService := otterizev1alpha2.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-services",
			Namespace: "test-namespace",
		},
		Spec: otterizev1alpha2.ProtectedServiceSpec{
			Name: "my-service",
		},
	}
	err := fakeValidator.validateNoDuplicateClients(&protectedService, &protectedServiceList)
	s.Require().Error(err)
}

func (s *ValidationWebhookTestSuite) TestValidateProtectedServicesFailIfUppercase() {
	fakeValidator := NewProtectedServiceValidator(nil)

	protectedServices := otterizev1alpha2.ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protected-services",
			Namespace: "test-namespace",
		},
		Spec: otterizev1alpha2.ProtectedServiceSpec{
			Name: "MyService",
		},
	}
	err := fakeValidator.validateSpec(&protectedServices)
	s.Require().Error(err)
}

func TestValidationWebhookTestSuite(t *testing.T) {
	suite.Run(t, new(ValidationWebhookTestSuite))
}
