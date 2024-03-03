package databaseconfigurator

//
//import (
//	"fmt"
//	"github.com/otterize/cloud/src/backend-service/pkg/lib/apis"
//	"github.com/otterize/cloud/src/backend-service/pkg/stores"
//	"github.com/stretchr/testify/suite"
//	"k8s.io/apimachinery/pkg/types"
//	"regexp"
//	"strings"
//	"testing"
//)
//
//type PostgresSuite struct {
//	suite.Suite
//}
//
//const testOrg = "organization"
//const testLongName = "this-is-a-very-long-name-verylong-verylong-verylong-verylong-verylong-verylong-verylong-verylong"
//const testShortName = "this-is-a-short_name"
//
//func (s *PostgresSuite) TestBuildPostgresUsernameShortName() {
//	types := []stores.ServiceType{stores.ServiceTypeKubernetes}
//	testCases := []string{testShortName, testLongName}
//
//	for _, tc := range testCases {
//		s.Run(tc, func() {
//			shortService, _ := stores.NewService(testOrg, tc, tc, types)
//			username := BuildPostgresUsername(shortService)
//			s.Require().True(strings.HasPrefix(username, fmt.Sprintf("%s_", shortService.ID)))
//			s.Require().LessOrEqual(len(username), pgUsernameMaxLength)
//		})
//	}
//}
//
//func (s *PostgresSuite) TestServiceIDFromPostgresUsername() {
//	types := []stores.ServiceType{stores.ServiceTypeKubernetes}
//	testCases := []string{testShortName, testLongName}
//
//	for _, tc := range testCases {
//		s.Run(tc, func() {
//			svc, _ := stores.NewService(testOrg, tc, tc, types)
//			username := BuildPostgresUsername(svc)
//			serviceID := ServiceIDFromPostgresUsername(username)
//			s.Require().Equal(serviceID, svc.ID)
//		})
//	}
//}
//
//type TestCaseTestPGUsernameHasOnlyAllowedChars struct {
//	name      string
//	namespace string
//}
//
//func (tc TestCaseTestPGUsernameHasOnlyAllowedChars) String() string {
//	return types.NamespacedName{
//		Namespace: tc.namespace,
//		Name:      tc.name,
//	}.String()
//}
//
//func (s *PostgresSuite) TestPGUsernameHasOnlyAllowedChars() {
//	testCases := []TestCaseTestPGUsernameHasOnlyAllowedChars{
//		{
//			name:      "my-service-name",
//			namespace: "my-namespace",
//		},
//	}
//
//	for _, tc := range testCases {
//		s.Run(tc.String(), func() {
//			namespace, _ := stores.NewNamespace(testOrg, apis.EnvironmentID("env-123"), apis.ClusterID("cluster-123"), tc.name)
//			svc, _ := stores.NewService(testOrg, tc.name, tc.name, []stores.ServiceType{stores.ServiceTypeKubernetes})
//			svc.Namespace = namespace
//			username := BuildPostgresUsername(svc)
//
//			// Names in SQL must begin with a letter (a-z) or underscore (_).
//			// Subsequent characters in a name can be letters, digits (0-9), or underscores.
//
//			matching := regexp.MustCompile(`^[a-z_][a-z0-9_]*$`).MatchString(username)
//			s.Require().Truef(matching, "Resulting username is not a legal postgres name: %s", username)
//		})
//	}
//}
//
//func TestRunTableNamesSuite(t *testing.T) {
//	suite.Run(t, new(PostgresSuite))
//}
