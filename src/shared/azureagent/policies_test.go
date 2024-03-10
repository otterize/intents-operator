package azureagent

import (
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/stretchr/testify/suite"
	"testing"
)

type AzureAgentPoliciesSuite struct {
	suite.Suite
}

func (s *AzureAgentPoliciesSuite) TestGetIntentScopeAppendSubscriptionID() {
	// Arrange
	agent := &Agent{
		conf: Config{
			SubscriptionID: "subscription-id",
		},
	}
	intent := otterizev1alpha3.Intent{
		Name: "/resourceGroups/resource-group-name/providers/Microsoft.Storage/storageAccounts/storage-account-name",
	}

	// Act
	scope, err := agent.getIntentScope(intent)

	// Assert
	s.Require().NoError(err)
	s.Equal("/subscriptions/subscription-id/resourceGroups/resource-group-name/providers/Microsoft.Storage/storageAccounts/storage-account-name", scope)
}

func (s *AzureAgentPoliciesSuite) TestGetIntentScopeAppendSubscriptionIDAndResourceGroup() {
	// Arrange
	agent := &Agent{
		conf: Config{
			SubscriptionID: "subscription-id",
			ResourceGroup:  "resource-group-name",
		},
	}
	intent := otterizev1alpha3.Intent{
		Name: "/providers/Microsoft.Storage/storageAccounts/storage-account-name",
	}

	// Act
	scope, err := agent.getIntentScope(intent)

	// Assert
	s.Require().NoError(err)
	s.Equal("/subscriptions/subscription-id/resourceGroups/resource-group-name/providers/Microsoft.Storage/storageAccounts/storage-account-name", scope)
}

func (s *AzureAgentPoliciesSuite) TestGetIntentScopeFullScope() {
	// Arrange
	agent := &Agent{}
	intent := otterizev1alpha3.Intent{
		Name: "/subscriptions/subscription-id/resourceGroups/resource-group-name/providers/Microsoft.Storage/storageAccounts/storage-account-name",
	}

	// Act
	scope, err := agent.getIntentScope(intent)

	// Assert
	s.Require().NoError(err)
	s.Equal("/subscriptions/subscription-id/resourceGroups/resource-group-name/providers/Microsoft.Storage/storageAccounts/storage-account-name", scope)
}

func (s *AzureAgentPoliciesSuite) TestGetIntentScopeError() {
	// Arrange
	agent := &Agent{}
	intent := otterizev1alpha3.Intent{
		Name: "invalid-scope",
	}

	// Act
	scope, err := agent.getIntentScope(intent)

	// Assert
	s.Require().Error(err)
	s.Empty(scope)
}

func TestAzureAgentPoliciesSuite(t *testing.T) {
	suite.Run(t, new(AzureAgentPoliciesSuite))
}
