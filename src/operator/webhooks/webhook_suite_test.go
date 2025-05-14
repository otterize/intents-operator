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
	otterizev1beta1 "github.com/otterize/intents-operator/src/operator/api/v1beta1"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	otterizev2beta1 "github.com/otterize/intents-operator/src/operator/api/v2beta1"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/enforcement"
	"github.com/otterize/intents-operator/src/shared/testbase"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	istiosecurityscheme "istio.io/client-go/pkg/apis/security/v1beta1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
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
	logrus.SetLevel(logrus.DebugLevel)
	s.TestEnv = &envtest.Environment{Scheme: scheme.Scheme}
	var err error
	s.TestEnv.CRDDirectoryPaths = []string{filepath.Join("..", "config", "crd")}
	s.TestEnv.WebhookInstallOptions = envtest.WebhookInstallOptions{
		Paths:            []string{filepath.Join("..", "config", "webhook")},
		LocalServingHost: "localhost",
	}
	utilruntime.Must(apiextensionsv1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(clientgoscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(istiosecurityscheme.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha2.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1alpha3.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev1beta1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev2alpha1.AddToScheme(s.TestEnv.Scheme))
	utilruntime.Must(otterizev2beta1.AddToScheme(s.TestEnv.Scheme))

	s.RestConfig, err = s.TestEnv.Start()
	s.Require().NoError(err)
	s.Require().NotNil(s.RestConfig)

	s.K8sDirectClient, err = kubernetes.NewForConfig(s.RestConfig)
	s.Require().NoError(err)
	s.Require().NotNil(s.K8sDirectClient)

}

func (s *ValidationWebhookTestSuite) SetupTest() {
	s.ControllerManagerTestSuiteBase.SetupTest()
	intentsValidator := NewIntentsValidatorV1alpha2(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha2.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator))
	intentsValidator13 := NewIntentsValidatorV1alpha3(s.Mgr.GetClient())
	s.Require().NoError((&otterizev1alpha3.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator13))
	intentsValidator2 := NewIntentsValidatorV2alpha1(s.Mgr.GetClient())
	s.Require().NoError((&otterizev2alpha1.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator2))
	intentsValidator2beta1 := NewIntentsValidatorV2beta1(s.Mgr.GetClient())
	s.Require().NoError((&otterizev2beta1.ClientIntents{}).SetupWebhookWithManager(s.Mgr, intentsValidator2beta1))
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

// Test Only one Target Type Can be used in a target
func (s *ValidationWebhookTestSuite) TestOnlyOneTargetTypeAllowed() {
	_, err := s.AddIntentsv2beta1("intents", "someclient", []otterizev2beta1.Target{
		{
			Kafka: &otterizev2beta1.KafkaTarget{
				Topics: []otterizev2beta1.KafkaTopic{{
					Name:       "sometopic",
					Operations: []otterizev2beta1.KafkaOperation{otterizev2beta1.KafkaOperationConsume},
				}},
			},
			Internet: &otterizev2beta1.Internet{
				Ips:   []string{"8.8.8.8"},
				Ports: []int{80},
			},
		},
	})
	s.Require().ErrorContains(err, "each target must have exactly one field set")

	_, err = s.AddIntentsv2beta1("intents", "someclient", []otterizev2beta1.Target{
		{
			AWS: &otterizev2beta1.AWSTarget{
				Actions: []string{"s3:GetObject"},
				ARN:     "arn:aws:s3:::mybucket/*",
			},
			Kubernetes: &otterizev2beta1.KubernetesTarget{Name: "omriservice", Kind: "Deployment"},
		},
	})
	s.Require().ErrorContains(err, "each target must have exactly one field set")

	_, err = s.AddIntentsv2beta1("intents", "someclient", []otterizev2beta1.Target{
		{
			Azure: &otterizev2beta1.AzureTarget{
				Roles: []string{"Contributor"},
				Scope: "subscriptions/1234-5678-9012-3456",
			},
			SQL: &otterizev2beta1.SQLTarget{
				Privileges: []otterizev2beta1.SQLPrivileges{{
					DatabaseName: "sadfsdf",
					Table:        "sometable",
					Operations:   []otterizev2beta1.DatabaseOperation{otterizev2beta1.DatabaseOperationSelect},
				}},
			},
		},
	})
	s.Require().ErrorContains(err, "each target must have exactly one field set")
}

func (s *ValidationWebhookTestSuite) TestNoDuplicateClientsAllowed() {
	_, err := s.AddIntentsv2beta1("intents", "someclient", []otterizev2beta1.Target{})
	s.Require().NoError(err)

	_, err = s.AddIntentsv2beta1("intents2", "someclient", []otterizev2beta1.Target{})
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
	_, err := s.AddIntentsv2beta1("kafka-intents", "kafka-client", []otterizev2beta1.Target{
		{
			Kafka: &otterizev2beta1.KafkaTarget{
				Topics: []otterizev2beta1.KafkaTopic{{
					Name:       "sometopic",
					Operations: []otterizev2beta1.KafkaOperation{otterizev2beta1.KafkaOperationConsume},
				}},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, missingNameFieldErr)

	_, err = s.AddIntentsv2beta1("http-intents", "http-client", []otterizev2beta1.Target{
		{
			Kubernetes: &otterizev2beta1.KubernetesTarget{
				HTTP: []otterizev2beta1.HTTPTarget{{
					Path:    "/somepath",
					Methods: []otterizev2beta1.HTTPMethod{otterizev2beta1.HTTPMethodGet},
				}},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, missingNameFieldErr)

	_, err = s.AddIntentsv2beta1("database-intents", "database-client", []otterizev2beta1.Target{
		{
			SQL: &otterizev2beta1.SQLTarget{
				Privileges: []otterizev2beta1.SQLPrivileges{{
					DatabaseName: "sadfsdf",
					Table:        "sometable",
					Operations:   []otterizev2beta1.DatabaseOperation{otterizev2beta1.DatabaseOperationSelect},
				}},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, missingNameFieldErr)

	_, err = s.AddIntentsv2beta1("aws-intents", "aws-client", []otterizev2beta1.Target{
		{
			AWS: &otterizev2beta1.AWSTarget{
				Actions: []string{"s3:GetObject"},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, strings.Replace(missingNameFieldErr, "name", "ARN", 1))

	_, err = s.AddIntentsv2beta1("azure-intents", "aws-client", []otterizev2beta1.Target{
		{
			Azure: &otterizev2beta1.AzureTarget{
				Roles: []string{"Contributor"},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, strings.Replace(missingNameFieldErr, "name", "scope", 1))

	_, err = s.AddIntentsv2beta1("gcp-intents", "aws-client", []otterizev2beta1.Target{
		{
			GCP: &otterizev2beta1.GCPTarget{
				Permissions: []string{"storage.objects.get"},
			},
		},
	})
	logrus.Infof("Error: %v", err)
	s.Require().ErrorContains(err, strings.Replace(missingNameFieldErr, "name", "resource", 1))

	_, err = s.AddIntentsv2beta1("internet-intents", "internet-client", []otterizev2beta1.Target{
		{
			Internet: &otterizev2beta1.Internet{
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

type ConversionWebhookTestSuite struct {
	suite.Suite
}

// Validate the conversion between v1alpha3 and v2beta1 works properly
func (s *ConversionWebhookTestSuite) TestConversionWebhookRegularIntents() {
	// Create a v1alpha3 object
	v1alpha3Intents := &otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "intents",
			Namespace: "test-namespace",
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: "name", Kind: "Depoloyment"},
			Calls: []otterizev1alpha3.Intent{
				{
					Name: "server",
				},
			},
		},
	}

	// Convert the v1alpha3 object to a v2beta1 object
	v2beta1Obj := &otterizev2beta1.ClientIntents{}
	err := v1alpha3Intents.ConvertTo(v2beta1Obj)
	s.Require().NoError(err)

	// Convert the v2beta1 object back to a v1alpha3 object
	anotherV1alpha3Intents := &otterizev1alpha3.ClientIntents{}
	err = anotherV1alpha3Intents.ConvertFrom(v2beta1Obj)
	s.Require().NoError(err)

	// Check that the two v1alpha3 objects are equal
	s.Require().Equal(v1alpha3Intents.GetClientKind(), anotherV1alpha3Intents.GetClientKind())
	s.Require().Equal(v1alpha3Intents.GetServiceName(), anotherV1alpha3Intents.GetServiceName())
	s.Require().Equal(len(v1alpha3Intents.GetCallsList()), len(anotherV1alpha3Intents.GetCallsList()))
	for i := range v1alpha3Intents.GetCallsList() {
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].GetServerFullyQualifiedName(v1alpha3Intents.GetNamespace()), anotherV1alpha3Intents.GetCallsList()[i].GetServerFullyQualifiedName(anotherV1alpha3Intents.GetNamespace()))
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].GetTargetServerKind(), anotherV1alpha3Intents.GetCallsList()[i].GetTargetServerKind())
	}
}

// Validate the conversion between v1alpha3 and v2beta1 works properly - kafka intents
func (s *ConversionWebhookTestSuite) TestConversionWebhookKafkaIntents() {
	// Create a v1alpha3 object
	v1alpha3Intents := &otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "intents",
			Namespace: "test-namespace",
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: "name", Kind: "Depoloyment"},
			Calls: []otterizev1alpha3.Intent{
				{
					Name: "server",
					Type: otterizev1alpha3.IntentTypeKafka,
					Topics: []otterizev1alpha3.KafkaTopic{
						{
							Name:       "sometopic",
							Operations: []otterizev1alpha3.KafkaOperation{otterizev1alpha3.KafkaOperationConsume, otterizev1alpha3.KafkaOperationProduce},
						},
						{
							Name:       "sometopic2",
							Operations: []otterizev1alpha3.KafkaOperation{otterizev1alpha3.KafkaOperationConsume},
						},
					},
				},
			},
		},
	}

	// Convert the v1alpha3 object to a v2beta1 object
	v2beta1Obj := &otterizev2beta1.ClientIntents{}
	err := v1alpha3Intents.ConvertTo(v2beta1Obj)
	s.Require().NoError(err)

	// Convert the v2beta1 object back to a v1alpha3 object
	anotherV1alpha3Intents := &otterizev1alpha3.ClientIntents{}
	err = anotherV1alpha3Intents.ConvertFrom(v2beta1Obj)
	s.Require().NoError(err)

	// Check that the two v1alpha3 objects are equal
	s.Require().Equal(v1alpha3Intents.GetClientKind(), anotherV1alpha3Intents.GetClientKind())
	s.Require().Equal(v1alpha3Intents.GetServiceName(), anotherV1alpha3Intents.GetServiceName())
	s.Require().Equal(len(v1alpha3Intents.GetCallsList()), len(anotherV1alpha3Intents.GetCallsList()))
	for i := range v1alpha3Intents.GetCallsList() {
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].GetServerFullyQualifiedName(v1alpha3Intents.GetNamespace()), anotherV1alpha3Intents.GetCallsList()[i].GetServerFullyQualifiedName(anotherV1alpha3Intents.GetNamespace()))
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].GetTargetServerKind(), anotherV1alpha3Intents.GetCallsList()[i].GetTargetServerKind())
		s.Require().Equal(len(v1alpha3Intents.GetCallsList()[i].Topics), len(anotherV1alpha3Intents.GetCallsList()[i].Topics))
		for j := range v1alpha3Intents.GetCallsList()[i].Topics {
			s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Topics[j].Name, anotherV1alpha3Intents.GetCallsList()[i].Topics[j].Name)
			s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Topics[j].Operations, anotherV1alpha3Intents.GetCallsList()[i].Topics[j].Operations)
		}
	}
}

// Validate the conversion between v1alpha3 and v2beta1 works properly - http intents
func (s *ConversionWebhookTestSuite) TestConversionWebhookHTTPIntents() {
	// Create a v1alpha3 object
	v1alpha3Intents := &otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "intents",
			Namespace: "test-namespace",
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: "name", Kind: "Depoloyment"},
			Calls: []otterizev1alpha3.Intent{
				{
					Name: "svc:server",
					Type: otterizev1alpha3.IntentTypeHTTP,
					HTTPResources: []otterizev1alpha3.HTTPResource{
						{
							Path:    "/somepath",
							Methods: []otterizev1alpha3.HTTPMethod{otterizev1alpha3.HTTPMethodGet, otterizev1alpha3.HTTPMethodPost},
						},
						{
							Path:    "/somepath2",
							Methods: []otterizev1alpha3.HTTPMethod{otterizev1alpha3.HTTPMethodGet},
						},
					},
				},
			},
		},
	}

	// Convert the v1alpha3 object to a v2beta1 object
	v2beta1Obj := &otterizev2beta1.ClientIntents{}
	err := v1alpha3Intents.ConvertTo(v2beta1Obj)
	s.Require().NoError(err)

	// Convert the v2beta1 object back to a v1alpha3 object
	anotherV1alpha3Intents := &otterizev1alpha3.ClientIntents{}
	err = anotherV1alpha3Intents.ConvertFrom(v2beta1Obj)
	s.Require().NoError(err)

	// Check that the two v1alpha3 objects are equal
	s.Require().Equal(v1alpha3Intents.GetClientKind(), anotherV1alpha3Intents.GetClientKind())
	s.Require().Equal(v1alpha3Intents.GetServiceName(), anotherV1alpha3Intents.GetServiceName())
	s.Require().Equal(len(v1alpha3Intents.GetCallsList()), len(anotherV1alpha3Intents.GetCallsList()))
	for i := range v1alpha3Intents.GetCallsList() {
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Type, anotherV1alpha3Intents.GetCallsList()[i].Type)
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].GetServerFullyQualifiedName(v1alpha3Intents.GetNamespace()), anotherV1alpha3Intents.GetCallsList()[i].GetServerFullyQualifiedName(anotherV1alpha3Intents.GetNamespace()))
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].GetTargetServerKind(), anotherV1alpha3Intents.GetCallsList()[i].GetTargetServerKind())
		s.Require().Equal(len(v1alpha3Intents.GetCallsList()[i].HTTPResources), len(anotherV1alpha3Intents.GetCallsList()[i].HTTPResources))
		for j := range v1alpha3Intents.GetCallsList()[i].HTTPResources {
			s.Require().Equal(v1alpha3Intents.GetCallsList()[i].HTTPResources[j].Path, anotherV1alpha3Intents.GetCallsList()[i].HTTPResources[j].Path)
			s.Require().Equal(v1alpha3Intents.GetCallsList()[i].HTTPResources[j].Methods, anotherV1alpha3Intents.GetCallsList()[i].HTTPResources[j].Methods)
		}
	}
}

// Validate the conversion between v1alpha3 and v2beta1 works properly - database intents
func (s *ConversionWebhookTestSuite) TestConversionWebhookDatabaseIntents() {
	// Create a v1alpha3 object
	v1alpha3Intents := &otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "intents",
			Namespace: "test-namespace",
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: "name", Kind: "Depoloyment"},
			Calls: []otterizev1alpha3.Intent{
				{
					Name: "server",
					Type: otterizev1alpha3.IntentTypeDatabase,
					DatabaseResources: []otterizev1alpha3.DatabaseResource{
						{
							DatabaseName: "somedb",
							Table:        "sometable",
							Operations:   []otterizev1alpha3.DatabaseOperation{otterizev1alpha3.DatabaseOperationSelect, otterizev1alpha3.DatabaseOperationInsert},
						},
						{
							DatabaseName: "somedb2",
							Table:        "sometable2",
							Operations:   []otterizev1alpha3.DatabaseOperation{otterizev1alpha3.DatabaseOperationSelect},
						},
					},
				},
			},
		},
	}

	// Convert the v1alpha3 object to a v2beta1 object
	v2beta1Obj := &otterizev2beta1.ClientIntents{}
	err := v1alpha3Intents.ConvertTo(v2beta1Obj)
	s.Require().NoError(err)

	// Convert the v2beta1 object back to a v1alpha3 object
	anotherV1alpha3Intents := &otterizev1alpha3.ClientIntents{}
	err = anotherV1alpha3Intents.ConvertFrom(v2beta1Obj)
	s.Require().NoError(err)

	// Check that the two v1alpha3 objects are equal
	s.Require().Equal(v1alpha3Intents.GetClientKind(), anotherV1alpha3Intents.GetClientKind())
	s.Require().Equal(v1alpha3Intents.GetServiceName(), anotherV1alpha3Intents.GetServiceName())
	s.Require().Equal(len(v1alpha3Intents.GetCallsList()), len(anotherV1alpha3Intents.GetCallsList()))
	for i := range v1alpha3Intents.GetCallsList() {
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Type, anotherV1alpha3Intents.GetCallsList()[i].Type)
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].GetServerFullyQualifiedName(v1alpha3Intents.GetNamespace()), anotherV1alpha3Intents.GetCallsList()[i].GetServerFullyQualifiedName(anotherV1alpha3Intents.GetNamespace()))
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].GetTargetServerKind(), anotherV1alpha3Intents.GetCallsList()[i].GetTargetServerKind())
		s.Require().Equal(len(v1alpha3Intents.GetCallsList()[i].DatabaseResources), len(anotherV1alpha3Intents.GetCallsList()[i].DatabaseResources))
		for j := range v1alpha3Intents.GetCallsList()[i].DatabaseResources {
			s.Require().Equal(v1alpha3Intents.GetCallsList()[i].DatabaseResources[j].DatabaseName, anotherV1alpha3Intents.GetCallsList()[i].DatabaseResources[j].DatabaseName)
			s.Require().Equal(v1alpha3Intents.GetCallsList()[i].DatabaseResources[j].Table, anotherV1alpha3Intents.GetCallsList()[i].DatabaseResources[j].Table)
			s.Require().Equal(v1alpha3Intents.GetCallsList()[i].DatabaseResources[j].Operations, anotherV1alpha3Intents.GetCallsList()[i].DatabaseResources[j].Operations)
		}
	}
}

// Validate the conversion between v1alpha3 and v2beta1 works properly - aws intents
func (s *ConversionWebhookTestSuite) TestConversionWebhookAWSIntents() {
	// Create a v1alpha3 object
	v1alpha3Intents := &otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "intents",
			Namespace: "test-namespace",
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: "name", Kind: "Depoloyment"},
			Calls: []otterizev1alpha3.Intent{
				{
					Name: "aen:server",
					Type: otterizev1alpha3.IntentTypeAWS,
					AWSActions: []string{
						"s3:GetObject",
						"s3:PutObject",
					},
				},
			},
		},
	}

	// Convert the v1alpha3 object to a v2beta1 object
	v2beta1Obj := &otterizev2beta1.ClientIntents{}
	err := v1alpha3Intents.ConvertTo(v2beta1Obj)
	s.Require().NoError(err)

	// Convert the v2beta1 object back to a v1alpha3 object
	anotherV1alpha3Intents := &otterizev1alpha3.ClientIntents{}
	err = anotherV1alpha3Intents.ConvertFrom(v2beta1Obj)
	s.Require().NoError(err)

	// Check that the two v1alpha3 objects are equal
	s.Require().Equal(v1alpha3Intents.GetClientKind(), anotherV1alpha3Intents.GetClientKind())
	s.Require().Equal(v1alpha3Intents.GetServiceName(), anotherV1alpha3Intents.GetServiceName())
	s.Require().Equal(len(v1alpha3Intents.GetCallsList()), len(anotherV1alpha3Intents.GetCallsList()))
	for i := range v1alpha3Intents.GetCallsList() {
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Type, anotherV1alpha3Intents.GetCallsList()[i].Type)
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Name, anotherV1alpha3Intents.GetCallsList()[i].Name)
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].AWSActions, anotherV1alpha3Intents.GetCallsList()[i].AWSActions)
	}
}

// Validate the conversion between v1alpha3 and v2beta1 works properly - azure intents
func (s *ConversionWebhookTestSuite) TestConversionWebhookAzureIntents() {
	// Create a v1alpha3 object
	v1alpha3Intents := &otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "intents",
			Namespace: "test-namespace",
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: "name", Kind: "Depoloyment"},
			Calls: []otterizev1alpha3.Intent{
				{
					Name: "aen:server",
					Type: otterizev1alpha3.IntentTypeAzure,
					AzureRoles: []string{
						"Contributor",
						"Reader",
					},
				},
			},
		},
	}

	// Convert the v1alpha3 object to a v2beta1 object
	v2beta1Obj := &otterizev2beta1.ClientIntents{}
	err := v1alpha3Intents.ConvertTo(v2beta1Obj)
	s.Require().NoError(err)

	// Convert the v2beta1 object back to a v1alpha3 object
	anotherV1alpha3Intents := &otterizev1alpha3.ClientIntents{}
	err = anotherV1alpha3Intents.ConvertFrom(v2beta1Obj)
	s.Require().NoError(err)

	// Check that the two v1alpha3 objects are equal
	s.Require().Equal(v1alpha3Intents.GetClientKind(), anotherV1alpha3Intents.GetClientKind())
	s.Require().Equal(v1alpha3Intents.GetServiceName(), anotherV1alpha3Intents.GetServiceName())
	s.Require().Equal(len(v1alpha3Intents.GetCallsList()), len(anotherV1alpha3Intents.GetCallsList()))
	for i := range v1alpha3Intents.GetCallsList() {
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Type, anotherV1alpha3Intents.GetCallsList()[i].Type)
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Name, anotherV1alpha3Intents.GetCallsList()[i].Name)
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].AzureRoles, anotherV1alpha3Intents.GetCallsList()[i].AzureRoles)
	}
}

// Validate the conversion between v1alpha3 and v2beta1 works properly - gcp intents
func (s *ConversionWebhookTestSuite) TestConversionWebhookGCPIntents() {
	// Create a v1alpha3 object
	v1alpha3Intents := &otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "intents",
			Namespace: "test-namespace",
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: "name", Kind: "Depoloyment"},
			Calls: []otterizev1alpha3.Intent{
				{
					Name: "aen:server",
					Type: otterizev1alpha3.IntentTypeGCP,
					GCPPermissions: []string{
						"storage.objects.get",
						"storage.objects.create",
					},
				},
			},
		},
	}

	// Convert the v1alpha3 object to a v2beta1 object
	v2beta1Obj := &otterizev2beta1.ClientIntents{}
	err := v1alpha3Intents.ConvertTo(v2beta1Obj)
	s.Require().NoError(err)

	// Convert the v2beta1 object back to a v1alpha3 object
	anotherV1alpha3Intents := &otterizev1alpha3.ClientIntents{}
	err = anotherV1alpha3Intents.ConvertFrom(v2beta1Obj)
	s.Require().NoError(err)

	// Check that the two v1alpha3 objects are equal
	s.Require().Equal(v1alpha3Intents.GetClientKind(), anotherV1alpha3Intents.GetClientKind())
	s.Require().Equal(v1alpha3Intents.GetServiceName(), anotherV1alpha3Intents.GetServiceName())
	s.Require().Equal(len(v1alpha3Intents.GetCallsList()), len(anotherV1alpha3Intents.GetCallsList()))
	for i := range v1alpha3Intents.GetCallsList() {
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Type, anotherV1alpha3Intents.GetCallsList()[i].Type)
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Name, anotherV1alpha3Intents.GetCallsList()[i].Name)
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].GCPPermissions, anotherV1alpha3Intents.GetCallsList()[i].GCPPermissions)
	}
}

// Validate the conversion between v1alpha3 and v2beta1 works properly - internet intents
func (s *ConversionWebhookTestSuite) TestConversionWebhookInternetIntents() {
	// Create a v1alpha3 object
	v1alpha3Intents := &otterizev1alpha3.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "intents",
			Namespace: "test-namespace",
		},
		Spec: &otterizev1alpha3.IntentsSpec{
			Service: otterizev1alpha3.Service{Name: "name", Kind: "Depoloyment"},
			Calls: []otterizev1alpha3.Intent{
				{
					Type: otterizev1alpha3.IntentTypeInternet,
					Internet: &otterizev1alpha3.Internet{
						Ips:   []string{"1.3.3.7", "122.133.144.155"},
						Ports: []int{80, 443},
					},
				},
				{
					Type: otterizev1alpha3.IntentTypeInternet,
					Internet: &otterizev1alpha3.Internet{
						Domains: []string{"www.google.com", "www.facebook.com"},
					},
				},
			},
		},
	}

	// Convert the v1alpha3 object to a v2beta1 object
	v2beta1Obj := &otterizev2beta1.ClientIntents{}
	err := v1alpha3Intents.ConvertTo(v2beta1Obj)
	s.Require().NoError(err)

	// Convert the v2beta1 object back to a v1alpha3 object
	anotherV1alpha3Intents := &otterizev1alpha3.ClientIntents{}
	err = anotherV1alpha3Intents.ConvertFrom(v2beta1Obj)
	s.Require().NoError(err)

	// Check that the two v1alpha3 objects are equal
	s.Require().Equal(v1alpha3Intents.GetClientKind(), anotherV1alpha3Intents.GetClientKind())
	s.Require().Equal(v1alpha3Intents.GetServiceName(), anotherV1alpha3Intents.GetServiceName())
	s.Require().Equal(len(v1alpha3Intents.GetCallsList()), len(anotherV1alpha3Intents.GetCallsList()))
	for i := range v1alpha3Intents.GetCallsList() {
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Type, anotherV1alpha3Intents.GetCallsList()[i].Type)
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Internet.Ips, anotherV1alpha3Intents.GetCallsList()[i].Internet.Ips)
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Internet.Ports, anotherV1alpha3Intents.GetCallsList()[i].Internet.Ports)
		s.Require().Equal(v1alpha3Intents.GetCallsList()[i].Internet.Domains, anotherV1alpha3Intents.GetCallsList()[i].Internet.Domains)
	}
}

func (s *ValidationWebhookTestSuite) TestStrictModeNonKubernetesServiceRejected() {
	viper.Set(enforcement.EnableStrictModeIntentsKey, true)
	_, err := s.AddIntentsInNamespaceV2beta1("test-intents", "test-client", s.TestNamespace, []otterizev2beta1.Target{
		{
			Kubernetes: &otterizev2beta1.KubernetesTarget{
				Name: "deny-me",
				Kind: "ReplicaSet",
			},
		},
	})
	s.Require().Error(err)
	s.Require().ErrorContains(err, "Target must be a Kubernetes service while in strict mode")
}

func (s *ValidationWebhookTestSuite) TestStrictModeWildcardDNSRejected() {
	viper.Set(enforcement.EnableStrictModeIntentsKey, true)
	_, err := s.AddIntentsInNamespaceV2beta1("test-intents", "test-client", s.TestNamespace, []otterizev2beta1.Target{
		{
			Internet: &otterizev2beta1.Internet{
				Domains: []string{"*.example.com"},
			},
		},
	})
	s.Require().Error(err)
	s.Require().ErrorContains(err, "must not contain wildcard domains while in strict mode")
}

func (s *ValidationWebhookTestSuite) TestStrictModeDNSWithoutPortRejected() {
	viper.Set(enforcement.EnableStrictModeIntentsKey, true)
	_, err := s.AddIntentsInNamespaceV2beta1("test-intents", "test-client", s.TestNamespace, []otterizev2beta1.Target{
		{
			Internet: &otterizev2beta1.Internet{
				Domains: []string{"api.example.com"},
				// Ports is not set
			},
		},
	})
	s.Require().Error(err)
	s.Require().ErrorContains(err, "must contain ports while in strict mode")
}

func TestValidationWebhookTestSuite(t *testing.T) {
	suite.Run(t, new(ValidationWebhookTestSuite))
}

func TestConversionWebhookTestSuite(t *testing.T) {
	suite.Run(t, new(ConversionWebhookTestSuite))
}
