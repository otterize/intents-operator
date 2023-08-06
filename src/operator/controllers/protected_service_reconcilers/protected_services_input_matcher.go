package protected_service_reconcilers

import (
	"fmt"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"go.uber.org/mock/gomock"
)

type ProtectedService struct {
	services []graphqlclient.ProtectedServiceInput
}

func (p ProtectedService) Matches(x interface{}) bool {
	services := x.([]graphqlclient.ProtectedServiceInput)
	if len(services) != len(p.services) {
		return false
	}

	for _, service := range services {
		found := false
		for _, expectedService := range p.services {
			if service.Name == expectedService.Name {
				found = true
				break
			}
		}

		if !found {
			return false
		}
	}

	return true
}

func (p ProtectedService) String() string {
	return fmt.Sprintf("%v", p.services)
}

func MatchProtectedServicesMatcher(protectedServices []graphqlclient.ProtectedServiceInput) gomock.Matcher {
	return ProtectedService{protectedServices}
}
