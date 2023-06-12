package kafkaacls

import (
	"github.com/Shopify/sarama"
	"github.com/golang/mock/gomock"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	mocks "github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/mocks"
	kafkaaclsmocks "github.com/otterize/intents-operator/src/operator/controllers/kafkaacls/mocks"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
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
	recorder         *record.FakeRecorder
	client           *mocks.MockClient
	intentsAdmin     KafkaIntentsAdmin
}

func (s *IntentAdminSuite) SetupTest() {
	controller := gomock.NewController(s.T())
	s.mockClusterAdmin = kafkaaclsmocks.NewMockClusterAdmin(controller)
	s.client = mocks.NewMockClient(controller)
	s.recorder = record.NewFakeRecorder(100)
}

func (s *IntentAdminSuite) TestApplyServerConfig() {
	topicName := "my-topic"
	kafkaServerConfig := otterizev1alpha2.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kafkaServerConfigResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev1alpha2.KafkaServerConfigSpec{
			Service: otterizev1alpha2.Service{
				Name: serverName,
			},
			Addr: serverAddress,
			Topics: []otterizev1alpha2.TopicConfig{
				{
					Topic:                  topicName,
					Pattern:                otterizev1alpha2.ResourcePatternTypeLiteral,
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
	kafkaServerConfig := otterizev1alpha2.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kafkaServerConfigResourceName,
			Namespace: testNamespace,
		},
		Spec: otterizev1alpha2.KafkaServerConfigSpec{
			Service: otterizev1alpha2.Service{
				Name: serverName,
			},
			Addr: serverAddress,
			Topics: []otterizev1alpha2.TopicConfig{
				{
					Topic:                  topicName,
					Pattern:                otterizev1alpha2.ResourcePatternTypeLiteral,
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
	kafkaServerConfig := otterizev1alpha2.KafkaServerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:              kafkaServerConfigResourceName,
			Namespace:         testNamespace,
			DeletionTimestamp: lo.ToPtr(metav1.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
		Spec: otterizev1alpha2.KafkaServerConfigSpec{
			Service: otterizev1alpha2.Service{
				Name: serverName,
			},
			Addr: serverAddress,
			Topics: []otterizev1alpha2.TopicConfig{
				{
					Topic:                  topicName,
					Pattern:                otterizev1alpha2.ResourcePatternTypeLiteral,
					ClientIdentityRequired: true,
					IntentsRequired:        false,
				},
			},
		},
	}

	s.intentsAdmin = NewKafkaIntentsAdminImpl(kafkaServerConfig, s.mockClusterAdmin, "user-name-mapping", true, true)

	aclDeleteFilterTopics := sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAny,
		Operation:                 sarama.AclOperationAny,
	}

	aclDeleteFilterOperatorGroup := sarama.AclFilter{
		ResourceType:              sarama.AclResourceGroup,
		ResourceName:              lo.ToPtr("*"),
		ResourcePatternTypeFilter: sarama.AclPatternLiteral,
		PermissionType:            sarama.AclPermissionAllow,
		Principal:                 lo.ToPtr(AnyUserPrincipalName),
		Operation:                 sarama.AclOperationAny,
	}

	allowAuthenticatedOnly := getAclAuthenticatedOnly(topicName, anonymousUsersPrincipal, allUsersPrincipal)
	operatorGroupPermission := getAclOperatorGroupPermission()

	topicAcl := []sarama.MatchingAcl{
		{
			Resource: allowAuthenticatedOnly.Resource,
			Acl:      *allowAuthenticatedOnly.Acls[0],
		},
		{
			Resource: allowAuthenticatedOnly.Resource,
			Acl:      *allowAuthenticatedOnly.Acls[1],
		},
	}
	groupAcl := []sarama.MatchingAcl{
		{
			Resource: operatorGroupPermission.Resource,
			Acl:      *operatorGroupPermission.Acls[0],
		},
		{
			Resource: operatorGroupPermission.Resource,
			Acl:      *operatorGroupPermission.Acls[1],
		},
	}
	gomock.InOrder(
		s.mockClusterAdmin.EXPECT().DeleteACL(aclDeleteFilterTopics, true).Return(topicAcl, nil),
		s.mockClusterAdmin.EXPECT().DeleteACL(aclDeleteFilterOperatorGroup, false).Return(groupAcl, nil),
	)

	err := s.intentsAdmin.RemoveAllIntents()
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
	s.expectNoEvent()
	s.intentsAdmin = nil
}

func (s *IntentAdminSuite) expectNoEvent() {
	select {
	case event := <-s.recorder.Events:
		s.Fail("Unexpected event found", event)
	default:
		// Amazing, no events left behind!
	}
}

func TestIntentAdminSuite(t *testing.T) {
	suite.Run(t, new(IntentAdminSuite))
}
