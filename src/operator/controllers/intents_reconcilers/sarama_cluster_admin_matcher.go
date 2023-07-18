package intents_reconcilers

import (
	"fmt"
	"github.com/Shopify/sarama"
	"go.uber.org/mock/gomock"
	"reflect"
)

type SaramaResource []*sarama.ResourceAcls

func (s *SaramaResource) String() string {
	resources := *s
	result := "SaramaResource"
	for _, resourceAclsPtr := range resources {
		if resourceAclsPtr == nil {
			continue
		}
		resourceAcls := *resourceAclsPtr
		result = fmt.Sprintf("{%s %+v", result, *resourceAclsPtr)
		for _, aclPtr := range resourceAcls.Acls {
			if aclPtr == nil {
				continue
			}
			result = fmt.Sprintf("%s %+v", result, *aclPtr)
		}
	}

	return fmt.Sprintf("%s}", result)
}

func (s *SaramaResource) Matches(arg interface{}) bool {
	expected := *s
	actual := reflect.ValueOf(arg).Interface().([]*sarama.ResourceAcls)

	if len(expected) != len(actual) {
		return false
	}

	for i := 0; i < len(expected); i++ {
		if expected[i] == nil && actual[i] == nil {
			continue
		} else if expected[i] == nil || actual[i] == nil {
			return false
		}

		a := *expected[i]
		b := *actual[i]

		if a.Resource != b.Resource || len(a.Acls) != len(b.Acls) {
			return false
		}

		for i := 0; i < len(a.Acls); i++ {
			if !reflect.DeepEqual(a.Acls[i], b.Acls[i]) {
				return false
			}
		}
	}

	return true
}

func MatchSaramaResource(s SaramaResource) gomock.Matcher {
	return &s
}
