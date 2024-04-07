package awspolicyagent

import (
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

func TestRunAWSAgentPolicySuite(t *testing.T) {
	suite.Run(t, new(AWSAgentPolicySuite))
}
