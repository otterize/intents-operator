package azureagent

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/stretchr/testify/suite"
	"testing"
)

type AzureAgentUtilsTestSuite struct {
	suite.Suite
}

type IsEqualTestCase struct {
	a        []*string
	b        []*string
	expected bool
}

var testCases = []IsEqualTestCase{
	{
		a:        []*string{to.Ptr("a"), to.Ptr("b"), to.Ptr("c")},
		b:        []*string{to.Ptr("a"), to.Ptr("b"), to.Ptr("c")},
		expected: true,
	},
	{
		a:        []*string{to.Ptr("a"), to.Ptr("b"), to.Ptr("c")},
		b:        []*string{to.Ptr("c"), to.Ptr("b"), to.Ptr("a")},
		expected: true,
	},
	{
		a:        []*string{to.Ptr("a"), to.Ptr("b"), to.Ptr("c")},
		b:        []*string{to.Ptr("a"), to.Ptr("b"), to.Ptr("d")},
		expected: false,
	},
	{
		a:        []*string{to.Ptr("a"), to.Ptr("b"), to.Ptr("c")},
		b:        []*string{to.Ptr("a"), to.Ptr("b")},
		expected: false,
	},
}

func (s *AzureAgentUtilsTestSuite) TestIsEqualAzureActions() {
	for _, testCase := range testCases {
		result := IsEqualAzureActions(testCase.a, testCase.b)
		s.Equal(testCase.expected, result)
	}
}

func TestAzureAgentUtilsSuite(t *testing.T) {
	suite.Run(t, new(AzureAgentUtilsTestSuite))
}
