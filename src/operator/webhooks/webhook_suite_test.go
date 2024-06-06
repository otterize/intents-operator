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
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/sirupsen/logrus"
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
	"strings"
	"testing"
	"time"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

type ValidationWebhookTestSuite struct {
	testbase.ControllerManagerTestSuiteBase
}

func (s *ValidationWebhookTestSuite) SetupSuite() {
	logrus.Info("Setting up test suite")
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
	utilruntime.Must(otterizev2alpha1.AddToScheme(s.TestEnv.Scheme))

}

func (s *ValidationWebhookTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	intentsValidator := NewIntentsValidatorV1alpha2(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha2.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator))
	intentsValidator13 := NewIntentsValidatorV1alpha3(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha3.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator13))
	intentsValidator2 := NewIntentsValidatorV2alpha1(s.Mgr.GetClient())
	s.Require().NoError((&otterizev2alpha1.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator2))
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
	_, err := s.AddIntentsv2alpha1("intents", "someclient", []otterizev2alpha1.Target{})
	s.Require().NoError(err)

	_, err = s.AddIntentsv2alpha1("intents2", "someclient", []otterizev2alpha1.Target{})
	s.Require().ErrorContains(err, "Intents for client someclient already exist in resource")
}

func (s *ValidationWebhookTestSuite) TestNoTopicsForHTTPIntents() {
	_, err := s.AddIntentsV1alpha2("intents", "someclient", []otterizev1alpha2.Intent{
		{
			Name: "server",
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
			Name: "server",
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
			Name: "server",
			Type: otterizev1alpha2.IntentTypeHTTP,
			Topics: []otterizev1alpha2.KafkaTopic{{
				Name:       "sometopic",
				Operations: []otterizev1alpha2.KafkaOperation{otterizev1alpha2.KafkaOperationConsume},
			}},
		},
	})
	s.Require().ErrorContains(err, expectedErr)
}

func (s *ValidationWebhookTestSuite) TestNameRequiredForEveryTypeExceptInternet() {
	missingNameFieldErr := "invalid intent format, field name is required"
	_, err := s.AddIntentsv2alpha1("kafka-intents", "kafka-client", []otterizev2alpha1.Target{
		{
			Kafka: &otterizev2alpha1.KafkaTarget{
				Topics: []otterizev2alpha1.KafkaTopic{{
					Name:       "sometopic",
					Operations: []otterizev2alpha1.KafkaOperation{otterizev2alpha1.KafkaOperationConsume},
				}},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, missingNameFieldErr)

	_, err = s.AddIntentsv2alpha1("http-intents", "http-client", []otterizev2alpha1.Target{
		{
			Kubernetes: &otterizev2alpha1.KubernetesTarget{
				HTTP: []otterizev2alpha1.HTTPTarget{{
					Path:    "/somepath",
					Methods: []otterizev2alpha1.HTTPMethod{otterizev2alpha1.HTTPMethodGet},
				}},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, missingNameFieldErr)

	_, err = s.AddIntentsv2alpha1("database-intents", "database-client", []otterizev2alpha1.Target{
		{
			SQL: &otterizev2alpha1.SQLTarget{
				Permissions: []otterizev2alpha1.SQLPermissions{{
					DatabaseName: "sadfsdf",
					Table:        "sometable",
					Operations:   []otterizev2alpha1.DatabaseOperation{otterizev2alpha1.DatabaseOperationSelect},
				}},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, missingNameFieldErr)

	_, err = s.AddIntentsv2alpha1("aws-intents", "aws-client", []otterizev2alpha1.Target{
		{
			AWS: &otterizev2alpha1.AWSTarget{
				Actions: []string{"s3:GetObject"},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, strings.Replace(missingNameFieldErr, "name", "ARN", 1))

	_, err = s.AddIntentsv2alpha1("azure-intents", "aws-client", []otterizev2alpha1.Target{
		{
			Azure: &otterizev2alpha1.AzureTarget{
				Roles: []string{"Contributor"},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, strings.Replace(missingNameFieldErr, "name", "scope", 1))

	_, err = s.AddIntentsv2alpha1("gcp-intents", "aws-client", []otterizev2alpha1.Target{
		{
			GCP: &otterizev2alpha1.GCPTarget{
				Permissions: []string{"storage.objects.get"},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, strings.Replace(missingNameFieldErr, "name", "resource", 1))

	_, err = s.AddIntentsv2alpha1("internet-intents", "internet-client", []otterizev2alpha1.Target{
		{
			Internet: &otterizev2alpha1.Internet{
				Ips:   []string{"1.1.1.1"},
				Ports: []int{80},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().NoError(err)
}

func (s *ValidationWebhookTestSuite) TestValidateProtectedServices() {
	fakeValidator := NewProtectedServiceValidatorV1alpha2(nil)

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
	fakeValidator := NewProtectedServiceValidatorV1alpha2(nil)

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
	fakeValidator := NewProtectedServiceValidatorV1alpha2(nil)

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
	fakeValidator := NewProtectedServiceValidatorV1alpha2(nil)

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
