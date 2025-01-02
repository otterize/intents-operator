package kafkaacls

import (
	"github.com/Shopify/sarama"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	kafkaaclsmocks "github.com/otterize/intents-operator/src/operator/controllers/kafkaacls/mocks"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

const (
	kafkaServerConfigResourceName = "kafka-server-config"
	testNamespace                 = "test-namespace"
	serverName                    = "kafka-server"
	serverAddress                 = "kafka-server.kafka:9092"
	anonymousUsersPrincipal       = "User:ANONYMOUS"
	allUsersPrincipal             = "User:*"
)

type IntentAdminSuite struct {
	suite.Suite
	mockClusterAdmin *kafkaaclsmocks.MockClusterAdmin
	intentsAdmin     KafkaIntentsAdmin
}

func (s *IntentAdminSuite) SetupTest() {
	controller := gomock.NewController(s.T())
	s.mockClusterAdmin = kafkaaclsmocks.NewMockClusterAdmin(controller)
}

func (s *IntentAdminSuite) TestApplyServerConfig() {
	topicName := "my-topic"
	kafkaServerConfig := otterizev2alpha1.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kafkaServerConfigResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev2alpha1.KafkaServerConfigSpec{
			Workload: otterizev2alpha1.Workload{
				Name: serverName,
			},
			Addr: serverAddress,
			Topics: []otterizev2alpha1.TopicConfig{
				{
					Topic:                  topicName,
					Pattern:                otterizev2alpha1.ResourcePatternTypeLiteral,
					ClientIdentityRequired: true,
					IntentsRequired:        false,
				},
			},
		},
	}

	s.intentsAdmin = NewKafkaIntentsAdminImpl(kafkaServerConfig, s.mockClusterAdmin, "user-name-mapping", true, true)
	aclListFilterAnonymous := sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAny,
		Operation:                 sarama.AclOperationAny,
		Principal:                 lo.ToPtr(anonymousUsersPrincipal),
	}
	aclListFilterAllUsers := sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAny,
		Operation:                 sarama.AclOperationAny,
		Principal:                 lo.ToPtr(allUsersPrincipal),
	}
	allowAuthenticatedOnly := getAclAuthenticatedOnly(topicName, anonymousUsersPrincipal, allUsersPrincipal)
	expectedACLsForTopic := []*sarama.ResourceAcls{
		&allowAuthenticatedOnly,
	}

	operatorGroupPermission := getAclOperatorGroupPermission()
	operatorACLForGroup := []*sarama.ResourceAcls{
		&operatorGroupPermission,
	}

	aclListFilterAll := sarama.AclFilter{
		ResourceType:              sarama.AclResourceAny,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAny,
		Operation:                 sarama.AclOperationAny,
	}
	gomock.InOrder(
		s.mockClusterAdmin.EXPECT().ListAcls(aclListFilterAnonymous).Return([]sarama.ResourceAcls{}, nil),
		s.mockClusterAdmin.EXPECT().ListAcls(aclListFilterAllUsers).Return([]sarama.ResourceAcls{}, nil),
		s.mockClusterAdmin.EXPECT().CreateACLs(MatchResourceAcls(expectedACLsForTopic)).Return(nil),
		s.mockClusterAdmin.EXPECT().CreateACLs(MatchResourceAcls(operatorACLForGroup)).Return(nil),
		s.mockClusterAdmin.EXPECT().ListAcls(aclListFilterAll).Return([]sarama.ResourceAcls{allowAuthenticatedOnly, operatorGroupPermission}, nil),
	)
	err := s.intentsAdmin.ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics)
	s.Require().NoError(err)
}

func (s *IntentAdminSuite) TestApplyServerConfigPermissionExists() {
	topicName := "my-topic"
	kafkaServerConfig := otterizev2alpha1.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kafkaServerConfigResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev2alpha1.KafkaServerConfigSpec{
			Workload: otterizev2alpha1.Workload{
				Name: serverName,
			},
			Addr: serverAddress,
			Topics: []otterizev2alpha1.TopicConfig{
				{
					Topic:                  topicName,
					Pattern:                otterizev2alpha1.ResourcePatternTypeLiteral,
					ClientIdentityRequired: true,
					IntentsRequired:        false,
				},
			},
		},
	}

	s.intentsAdmin = NewKafkaIntentsAdminImpl(kafkaServerConfig, s.mockClusterAdmin, "user-name-mapping", true, true)

	aclListFilterAnonymous := sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAny,
		Operation:                 sarama.AclOperationAny,
		Principal:                 lo.ToPtr(anonymousUsersPrincipal),
	}
	aclListFilterAllUsers := sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAny,
		Operation:                 sarama.AclOperationAny,
		Principal:                 lo.ToPtr(allUsersPrincipal),
	}

	allowAuthenticatedOnly := getAclAuthenticatedOnly(topicName, anonymousUsersPrincipal, allUsersPrincipal)
	operatorGroupPermission := getAclOperatorGroupPermission()
	operatorACLForGroup := []*sarama.ResourceAcls{
		&operatorGroupPermission,
	}
	aclListFilterAllPrincipals := sarama.AclFilter{
		ResourceType:              sarama.AclResourceAny,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAny,
		Operation:                 sarama.AclOperationAny,
	}

	existingAnonymousAcl := sarama.ResourceAcls{
		Resource: sarama.Resource{
			ResourceType:        sarama.AclResourceTopic,
			ResourceName:        topicName,
			ResourcePatternType: sarama.AclPatternLiteral,
		},
		Acls: []*sarama.Acl{
			{
				Principal:      anonymousUsersPrincipal,
				Host:           "*",
				Operation:      sarama.AclOperationAll,
				PermissionType: sarama.AclPermissionDeny,
			},
		},
	}
	existingAllUsersAcl := sarama.ResourceAcls{
		Resource: sarama.Resource{
			ResourceType:        sarama.AclResourceTopic,
			ResourceName:        topicName,
			ResourcePatternType: sarama.AclPatternLiteral,
		},
		Acls: []*sarama.Acl{
			{
				Principal:      allUsersPrincipal,
				Host:           "*",
				Operation:      sarama.AclOperationAll,
				PermissionType: sarama.AclPermissionAllow,
			},
		},
	}

	gomock.InOrder(
		s.mockClusterAdmin.EXPECT().ListAcls(aclListFilterAnonymous).Return([]sarama.ResourceAcls{existingAnonymousAcl}, nil),
		s.mockClusterAdmin.EXPECT().ListAcls(aclListFilterAllUsers).Return([]sarama.ResourceAcls{existingAllUsersAcl}, nil),
		s.mockClusterAdmin.EXPECT().CreateACLs(MatchResourceAcls(operatorACLForGroup)).Return(nil),
		s.mockClusterAdmin.EXPECT().ListAcls(aclListFilterAllPrincipals).Return([]sarama.ResourceAcls{allowAuthenticatedOnly, operatorGroupPermission}, nil),
	)

	err := s.intentsAdmin.ApplyServerTopicsConf(kafkaServerConfig.Spec.Topics)
	s.Require().NoError(err)
}

func (s *IntentAdminSuite) TestDeleteServerConfig() {
	topicName := "my-topic"
	kafkaServerConfig := otterizev2alpha1.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:              kafkaServerConfigResourceName,
			Namespace:         testNamespace,
			DeletionTimestamp: lo.ToPtr(metav1.Date(2021, 6, 13, 0, 0, 0, 0, time.UTC)),
		},
		Spec: otterizev2alpha1.KafkaServerConfigSpec{
			Workload: otterizev2alpha1.Workload{
				Name: serverName,
			},
			Addr: serverAddress,
			Topics: []otterizev2alpha1.TopicConfig{
				{
					Topic:                  topicName,
					Pattern:                otterizev2alpha1.ResourcePatternTypeLiteral,
					ClientIdentityRequired: true,
					IntentsRequired:        false,
				},
			},
		},
	}

	s.intentsAdmin = NewKafkaIntentsAdminImpl(kafkaServerConfig, s.mockClusterAdmin, "user-name-mapping", true, true)

	resource := sarama.Resource{
		ResourceType:        sarama.AclResourceTopic,
		ResourceName:        topicName,
		ResourcePatternType: sarama.AclPatternLiteral,
	}

	anonymousUsersTopicAcl := []sarama.MatchingAcl{
		{
			Resource: resource,
			Acl: sarama.Acl{
				Principal:      anonymousUsersPrincipal,
				Host:           "*",
				Operation:      sarama.AclOperationAll,
				PermissionType: sarama.AclPermissionDeny,
			},
		},
	}
	authenticatedUsersTopicAcl := []sarama.MatchingAcl{
		{
			Resource: resource,
			Acl: sarama.Acl{
				Principal:      allUsersPrincipal,
				Host:           "*",
				Operation:      sarama.AclOperationAll,
				PermissionType: sarama.AclPermissionAllow,
			},
		},
	}

	groupAcl := []sarama.MatchingAcl{
		{
			Resource: resource,
			Acl: sarama.Acl{
				Principal:      allUsersPrincipal,
				Host:           "*",
				Operation:      sarama.AclOperationRead,
				PermissionType: sarama.AclPermissionAllow,
			},
		},
		{
			Resource: resource,
			Acl: sarama.Acl{
				Principal:      allUsersPrincipal,
				Host:           "*",
				Operation:      sarama.AclOperationDescribe,
				PermissionType: sarama.AclPermissionAllow,
			},
		},
	}

	anonymousUserACLFilter := sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourceName:              lo.ToPtr("my-topic"),
		ResourcePatternTypeFilter: sarama.AclPatternLiteral,
		Principal:                 lo.ToPtr("User:ANONYMOUS"),
		Host:                      lo.ToPtr("*"),
		Operation:                 sarama.AclOperationAll,
		PermissionType:            sarama.AclPermissionDeny,
	}
	authenticatedUserACLFilter := sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourceName:              lo.ToPtr("my-topic"),
		ResourcePatternTypeFilter: sarama.AclPatternLiteral,
		Principal:                 lo.ToPtr("User:*"),
		Host:                      lo.ToPtr("*"),
		Operation:                 sarama.AclOperationAll,
		PermissionType:            sarama.AclPermissionAllow,
	}
	aclDeleteFilterOperatorGroup := sarama.AclFilter{
		ResourceType:              sarama.AclResourceGroup,
		ResourceName:              lo.ToPtr("*"),
		ResourcePatternTypeFilter: sarama.AclPatternLiteral,
		PermissionType:            sarama.AclPermissionAllow,
		Principal:                 lo.ToPtr(AnyUserPrincipalName),
		Operation:                 sarama.AclOperationAny,
	}

	s.mockClusterAdmin.EXPECT().DeleteACL(anonymousUserACLFilter, false).Return(anonymousUsersTopicAcl, nil)
	s.mockClusterAdmin.EXPECT().DeleteACL(authenticatedUserACLFilter, false).Return(authenticatedUsersTopicAcl, nil)
	s.mockClusterAdmin.EXPECT().DeleteACL(aclDeleteFilterOperatorGroup, false).Return(groupAcl, nil)

	err := s.intentsAdmin.RemoveServerIntents(kafkaServerConfig.Spec.Topics)
	s.Require().NoError(err)
}

func getAclOperatorGroupPermission() sarama.ResourceAcls {
	return sarama.ResourceAcls{
		Resource: sarama.Resource{
			ResourceType:        sarama.AclResourceGroup,
			ResourceName:        "*",
			ResourcePatternType: sarama.AclPatternLiteral,
		},
		Acls: []*sarama.Acl{
			{
				Principal:      allUsersPrincipal,
				Host:           "*",
				Operation:      sarama.AclOperationRead,
				PermissionType: sarama.AclPermissionAllow,
			},
			{
				Principal:      allUsersPrincipal,
				Host:           "*",
				Operation:      sarama.AclOperationDescribe,
				PermissionType: sarama.AclPermissionAllow,
			},
		},
	}
}

func getAclAuthenticatedOnly(topicName string, anonymousUsersPrincipal string, allUsersPrincipal string) sarama.ResourceAcls {
	return sarama.ResourceAcls{
		Resource: sarama.Resource{
			ResourceType:        sarama.AclResourceTopic,
			ResourceName:        topicName,
			ResourcePatternType: sarama.AclPatternLiteral,
		},
		Acls: []*sarama.Acl{
			{
				Principal:      anonymousUsersPrincipal,
				Host:           "*",
				Operation:      sarama.AclOperationAll,
				PermissionType: sarama.AclPermissionDeny,
			},
			{
				Principal:      allUsersPrincipal,
				Host:           "*",
				Operation:      sarama.AclOperationAll,
				PermissionType: sarama.AclPermissionAllow,
			},
		},
	}
}

func (s *IntentAdminSuite) TearDownTest() {
	s.intentsAdmin = nil
}

func TestIntentAdminSuite(t *testing.T) {
	suite.Run(t, new(IntentAdminSuite))
}
