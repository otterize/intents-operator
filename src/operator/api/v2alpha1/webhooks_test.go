package v2alpha1

import (
	"github.com/otterize/intents-operator/src/operator/api/v2beta1"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

type WebhooksTestSuite struct {
	suite.Suite
}

func (t *WebhooksTestSuite) TestMySQLServerConfigConversion() {
	// Create a MySQLServerConfig with random data
	original := &MySQLServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: MySQLServerConfigSpec{
			Address: "testAddress",
			Credentials: DatabaseCredentials{
				Username: "testUsername",
				Password: "testPassword",
				SecretRef: &DatabaseCredentialsSecretRef{
					Name:        "testName",
					Namespace:   "testNamespace",
					UsernameKey: "testUsernameKey",
					PasswordKey: "testPasswordKey",
				},
			},
		},
	}

	// ConvertTo
	dstRaw := &v2beta1.MySQLServerConfig{}
	err := original.ConvertTo(dstRaw)
	t.Require().NoError(err)

	// ConvertFrom
	converted := &MySQLServerConfig{}
	err = converted.ConvertFrom(dstRaw)
	t.Require().NoError(err)

	t.Require().Equal(original.Spec, converted.Spec)

}

// PostgreSQLServerConfig test
func (t *WebhooksTestSuite) TestPostgreSQLServerConfigConversion() {
	// Create a PostgreSQLServerConfig with random data
	original := &PostgreSQLServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: PostgreSQLServerConfigSpec{
			Address: "testAddress",
			Credentials: DatabaseCredentials{
				Username: "testUsername",
				Password: "testPassword",
				SecretRef: &DatabaseCredentialsSecretRef{
					Name:        "testName",
					Namespace:   "testNamespace",
					UsernameKey: "testUsernameKey",
					PasswordKey: "testPasswordKey",
				},
			},
		},
	}

	// ConvertTo
	dstRaw := &v2beta1.PostgreSQLServerConfig{}
	err := original.ConvertTo(dstRaw)
	t.Require().NoError(err)

	// ConvertFrom
	converted := &PostgreSQLServerConfig{}
	err = converted.ConvertFrom(dstRaw)
	t.Require().NoError(err)

	t.Require().Equal(original.Spec, converted.Spec)
}

// KafkaServerConfig test
func (t *WebhooksTestSuite) TestKafkaServerConfigConversion() {
	// Create a KafkaServerConfig with random data
	original := &KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: KafkaServerConfigSpec{
			// random data for tests
			Workload: Workload{
				Name: "test",
				Kind: "test",
			},
			NoAutoCreateIntentsForOperator: true,
			Addr:                           "test",
			TLS:                            TLSSource{CertFile: "sa", KeyFile: "aa", RootCAFile: "aa"},
			Topics: []TopicConfig{{
				Topic: "test",
			},
			}},
	}

	// ConvertTo
	dstRaw := &v2beta1.KafkaServerConfig{}
	err := original.ConvertTo(dstRaw)
	t.Require().NoError(err)

	// ConvertFrom
	converted := &KafkaServerConfig{}
	err = converted.ConvertFrom(dstRaw)
	t.Require().NoError(err)

	t.Require().Equal(original.Spec, converted.Spec)
}

// ProtectedService test
func (t *WebhooksTestSuite) TestProtectedServiceConversion() {
	// Create a ProtectedService with random data
	original := &ProtectedService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: ProtectedServiceSpec{
			Name: "test",
			Kind: "Deployment",
		},
	}

	// ConvertTo
	dstRaw := &v2beta1.ProtectedService{}
	err := original.ConvertTo(dstRaw)
	t.Require().NoError(err)

	// ConvertFrom
	converted := &ProtectedService{}
	err = converted.ConvertFrom(dstRaw)
	t.Require().NoError(err)

	t.Require().Equal(original.Spec, converted.Spec)
}

func (t *WebhooksTestSuite) TestClientIntentsKubernetes() {
	// Create a ClientIntents with random data
	original := &ClientIntents{
		Status: ClientIntentsStatus{ObservedGeneration: 1, UpToDate: true, ResolvedIPs: []ResolvedIPs{{DNS: "a.test", IPs: []string{"1.3.3.7"}}}},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: &IntentsSpec{
			Workload: Workload{
				Name: "test",
				Kind: "test",
			},
			Targets: []Target{
				{
					Kubernetes: &KubernetesTarget{
						Name: "test.test",
					},
				},
				{
					Kubernetes: &KubernetesTarget{
						Name: "test2",
						Kind: "Deployment",
					},
				},
				{
					Kubernetes: &KubernetesTarget{
						Name: "test3.other-namespace",
						Kind: "Deployment",
					},
				},
				{
					Service: &ServiceTarget{
						Name: "kubernetes.default",
					},
				},
			},
		}}

	// ConvertTo
	dstRaw := &v2beta1.ClientIntents{}
	err := original.ConvertTo(dstRaw)
	t.Require().NoError(err)

	// ConvertFrom
	converted := &ClientIntents{}
	err = converted.ConvertFrom(dstRaw)
	t.Require().NoError(err)

	t.Require().Equal(original.Spec, converted.Spec)
	t.Require().Equal(original.Status, converted.Status)
}

func (t *WebhooksTestSuite) TestClientIntentsAzureActionsDataActions() {
	// Create a ClientIntents with random data
	original := &ClientIntents{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: &IntentsSpec{
			Workload: Workload{
				Name: "test",
			},
			Targets: []Target{
				{
					Azure: &AzureTarget{
						Scope: "testscope1",
						DataActions: []AzureDataAction{
							"testDataAction1",
							"testDataAction2",
						},
					},
				},
				{
					Azure: &AzureTarget{
						Scope: "testscope2",
						Actions: []AzureAction{
							"testAction1",
							"testAction2",
						},
					},
				},
			},
		},
	}

	// ConvertTo
	dstRaw := &v2beta1.ClientIntents{}
	err := original.ConvertTo(dstRaw)
	t.Require().NoError(err)

	// ConvertFrom
	converted := &ClientIntents{}
	err = converted.ConvertFrom(dstRaw)
	t.Require().NoError(err)

	t.Require().Equal(original.Spec, converted.Spec)
}

func TestWebhooksTestSuite(t *testing.T) {
	suite.Run(t, new(WebhooksTestSuite))
}
