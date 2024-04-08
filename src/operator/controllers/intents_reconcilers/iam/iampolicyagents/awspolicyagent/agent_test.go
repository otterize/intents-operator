package awspolicyagent

import (
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/stretchr/testify/suite"
	"testing"
)

type AWSAgentPolicySuite struct {
	suite.Suite
}

// Test templateResourceName
func (s *AWSAgentPolicySuite) Test_templateResourceName() {
	// Given
	agent := &Agent{
		Agent: &awsagent.Agent{
			Region:    "test-region",
			AccountID: "test-accountid",
		},
	}
	// When
	resourceName := agent.templateResourceName("arn:aws:sqs:$(AWS_REGION):$(AWS_ACCOUNT_ID):queue1")
	// Then
	s.Equal("arn:aws:sqs:test-region:test-accountid:queue1", resourceName)
}

func (a *AWSAgentPolicySuite) Test_createPolicyFromIntents_TemplateResourceName() {
	// Given
	agent := &Agent{
		Agent: &awsagent.Agent{
			Region:    "test-region",
			AccountID: "test-accountid",
		},
	}
	intents := []otterizev1alpha3.Intent{
		{
			Name:       "arn:aws:sqs:$(AWS_REGION):$(AWS_ACCOUNT_ID):queue1",
			AWSActions: []string{"sqs:SendMessage"},
		},
	}
	// When
	policyDoc := agent.createPolicyFromIntents(intents)
	// Then
	a.Equal("arn:aws:sqs:test-region:test-accountid:queue1", policyDoc.Statement[0].Resource)

}

func TestRunAWSAgentPolicySuite(t *testing.T) {
	suite.Run(t, new(AWSAgentPolicySuite))
}
