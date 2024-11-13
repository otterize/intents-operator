package awspolicyagent

import (
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
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
		agent: &awsagent.Agent{
			Region:    "test-region",
			AccountID: "test-accountid",
		},
	}
	// When
	resourceName := agent.templateResourceName("arn:aws:sqs:$(AWS_REGION):$(AWS_ACCOUNT_ID):queue1")
	// Then
	s.Equal("arn:aws:sqs:test-region:test-accountid:queue1", resourceName)
}

func (s *AWSAgentPolicySuite) Test_createPolicyFromIntents_TemplateResourceName() {
	// Given
	agent := &Agent{
		agent: &awsagent.Agent{
			Region:    "test-region",
			AccountID: "test-accountid",
		},
	}
	intents := []otterizev2.Target{
		{
			AWS: &otterizev2.AWSTarget{
				ARN:     "arn:aws:sqs:$(AWS_REGION):$(AWS_ACCOUNT_ID):queue1",
				Actions: []string{"sqs:SendMessage"},
			},
		},
	}
	// When
	policyDoc := agent.createPolicyFromIntents(intents)
	// Then
	s.Equal("arn:aws:sqs:test-region:test-accountid:queue1", policyDoc.Statement[0].Resource)

}

func (s *AWSAgentPolicySuite) TestCreatePolicyFromIntents_NoStatements() {
	// Given
	agent := &Agent{
		agent: &awsagent.Agent{
			Region:    "test-region",
			AccountID: "test-accountid",
		},
	}
	var intents []otterizev2.Target
	// When
	policyDoc := agent.createPolicyFromIntents(intents)
	// Then
	s.Len(policyDoc.Statement, 1)
	s.Equal("*", policyDoc.Statement[0].Resource)
	s.Equal([]string{"none:null"}, policyDoc.Statement[0].Action)
}

func TestRunAWSAgentPolicySuite(t *testing.T) {
	suite.Run(t, new(AWSAgentPolicySuite))
}
