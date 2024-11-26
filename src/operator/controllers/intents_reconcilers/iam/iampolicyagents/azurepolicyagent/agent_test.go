package azurepolicyagent

import (
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	"github.com/stretchr/testify/suite"
	"sync"
	"testing"
)

const (
	testSubscriptionID = "test-subscriptionid"
	testResourceGroup  = "test-resourcegroup"
	testAKSClusterName = "test-aksclustername"
	testTenantID       = "test-tenantid"
	testLocation       = "test-location"
	testOIDCIssuerURL  = "test-oidcissuerurl"

	testNamespace          = "test-namespace"
	testAccountName        = "test-accountname"
	testIntentsServiceName = "test-intentsservicename"

	testKeyVaultName = "test-keyvaultname"
)

type AzureAgentPoliciesSuite struct {
	suite.Suite
	agent *Agent
}

func (s *AzureAgentPoliciesSuite) SetupTest() {
	s.agent = &Agent{
		&azureagent.Agent{
			Conf: azureagent.Config{
				SubscriptionID: testSubscriptionID,
				ResourceGroup:  testResourceGroup,
			},
		},
		sync.Mutex{},
		sync.Mutex{},
	}
}

type GetIntentScopeTestCase struct {
	Name          string
	IntentName    string
	ExpectedScope string
}

var getIntentScopeTestCases = []GetIntentScopeTestCase{
	{
		Name:          "AppendSubscriptionID",
		IntentName:    "/resourceGroups/resourcegroup/providers/Microsoft.Storage/storageAccounts/storage-account-name",
		ExpectedScope: "/subscriptions/test-subscriptionid/resourceGroups/resourcegroup/providers/Microsoft.Storage/storageAccounts/storage-account-name",
	},
	{
		Name:          "AppendSubscriptionIDAndResourceGroup",
		IntentName:    "/providers/Microsoft.Storage/storageAccounts/storage-account-name",
		ExpectedScope: "/subscriptions/test-subscriptionid/resourceGroups/test-resourcegroup/providers/Microsoft.Storage/storageAccounts/storage-account-name",
	},
	{
		Name:          "IntentIsFullScope",
		IntentName:    "/subscriptions/subscriptionid/resourceGroups/resourcegroup/providers/Microsoft.Storage/storageAccounts/storage-account-name",
		ExpectedScope: "/subscriptions/subscriptionid/resourceGroups/resourcegroup/providers/Microsoft.Storage/storageAccounts/storage-account-name",
	},
}

func (s *AzureAgentPoliciesSuite) TestGetIntentScope_HappyFlows() {
	for _, testCase := range getIntentScopeTestCases {
		s.Run(testCase.Name, func() {
			// Arrange
			intent := otterizev2alpha1.Target{
				Azure: &otterizev2alpha1.AzureTarget{
					Scope: testCase.IntentName,
				},
			}

			// Act
			scope, err := s.agent.getIntentScope(intent)

			// Assert
			s.Require().NoError(err)
			s.Equal(testCase.ExpectedScope, scope)
		})
	}
}

func (s *AzureAgentPoliciesSuite) TestGetIntentScopeError() {
	// Arrange
	intent := otterizev2alpha1.Target{
		Azure: &otterizev2alpha1.AzureTarget{
			// Invalid scope
			Scope: "invalid-scope",
		},
	}

	// Act
	scope, err := s.agent.getIntentScope(intent)

	// Assert
	s.Require().Error(err)
	s.Empty(scope)
}

func TestAzureAgentPoliciesSuite(t *testing.T) {
	suite.Run(t, new(AzureAgentPoliciesSuite))
}
