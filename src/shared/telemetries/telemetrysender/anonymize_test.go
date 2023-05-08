package telemetrysender

import (
	"crypto/sha256"
	"fmt"
	"github.com/stretchr/testify/suite"
	"testing"
)

type TelemetrySenderTestSuite struct {
	suite.Suite
}

func (t *TelemetrySenderTestSuite) TestAnonymizeNotContainsOriginalStr() {
	someStr := "This is a test string"
	hash := Anonymize(someStr)
	t.Require().NotContains(someStr, hash)
	t.Require().NotContains(hash, someStr)
}

func (t *TelemetrySenderTestSuite) TestAnonymizeDeterministic() {
	someStr := "This is a test string"
	hash := Anonymize(someStr)
	hash2 := Anonymize(someStr)
	t.Require().Equal(hash, hash2)
}

func (t *TelemetrySenderTestSuite) TestAnonymizeDifferentForDifferentInputs() {
	someStr := "This is a test string"
	someStr2 := "This is a completely different test string"
	hash := Anonymize(someStr)
	hash2 := Anonymize(someStr2)
	t.Require().NotEqual(hash, hash2)
}

func (t *TelemetrySenderTestSuite) TestAnonymizeSalted() {
	someStr := "This is a test string"
	hash := Anonymize(someStr)
	hash2 := fmt.Sprintf("%x", sha256.Sum256([]byte(someStr)))
	t.Require().NotEqual(hash, hash2)
}

func TestTelemetrySenderTestSuite(t *testing.T) {
	suite.Run(t, &TelemetrySenderTestSuite{})
}
