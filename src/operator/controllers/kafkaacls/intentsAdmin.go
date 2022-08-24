package kafkaacls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/Shopify/sarama"
	otterizev1alpha1 "github.com/otterize/intents-operator/shared/api/v1alpha1"
	"github.com/otterize/lox"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/vishalkuo/bimap"
	"io/ioutil"
	"log"
	"os"
)

type TopicToACLList map[string][]*sarama.Acl

const (
	AnonymousUserPrincipalName = "User:ANONYMOUS"
	AnyUserPrincipalName       = "User:*"
)

type KafkaIntentsAdmin struct {
	kafkaServer      otterizev1alpha1.KafkaServerConfig
	kafkaAdminClient sarama.ClusterAdmin
}

var (
	kafkaOperationToAclOperation = map[otterizev1alpha1.KafkaOperation]sarama.AclOperation{
		otterizev1alpha1.KafkaOperationConsume:         sarama.AclOperationRead,
		otterizev1alpha1.KafkaOperationProduce:         sarama.AclOperationWrite,
		otterizev1alpha1.KafkaOperationCreate:          sarama.AclOperationCreate,
		otterizev1alpha1.KafkaOperationDelete:          sarama.AclOperationDelete,
		otterizev1alpha1.KafkaOperationAlter:           sarama.AclOperationAlter,
		otterizev1alpha1.KafkaOperationDescribe:        sarama.AclOperationDescribe,
		otterizev1alpha1.KafkaOperationClusterAction:   sarama.AclOperationClusterAction,
		otterizev1alpha1.KafkaOperationDescribeConfigs: sarama.AclOperationDescribeConfigs,
		otterizev1alpha1.KafkaOperationAlterConfigs:    sarama.AclOperationAlterConfigs,
		otterizev1alpha1.KafkaOperationIdempotentWrite: sarama.AclOperationIdempotentWrite,
	}
	KafkaOperationToAclOperationBMap = bimap.NewBiMapFromMap(kafkaOperationToAclOperation)

	kafkaPatternTypeToSaramaPatternType = map[otterizev1alpha1.ResourcePatternType]sarama.AclResourcePatternType{
		otterizev1alpha1.ResourcePatternTypeLiteral: sarama.AclPatternLiteral,
		otterizev1alpha1.ResourcePatternTypePrefix:  sarama.AclPatternPrefixed,
	}
)

func getTLSConfig(tlsSource otterizev1alpha1.TLSSource) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(tlsSource.CertFile, tlsSource.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed loading x509 key pair: %w", err)
	}

	pool := x509.NewCertPool()
	rootCAPEM, err := ioutil.ReadFile(tlsSource.RootCAFile)
	if err != nil {
		return nil, fmt.Errorf("failed loading root CA PEM file: %w ", err)
	}
	pool.AppendCertsFromPEM(rootCAPEM)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	}, nil
}

func NewKafkaIntentsAdmin(kafkaServer otterizev1alpha1.KafkaServerConfig) (*KafkaIntentsAdmin, error) {
	logger := logrus.WithField("addr", kafkaServer.Spec.Addr)
	logger.Info("Connecting to kafka server")
	addrs := []string{kafkaServer.Spec.Addr}

	config := sarama.NewConfig()
	config.Version = sarama.V2_0_0_0

	tlsConfig, err := getTLSConfig(kafkaServer.Spec.TLS)
	if err != nil {
		return nil, err
	}

	config.Net.TLS.Config = tlsConfig
	config.Net.TLS.Enable = true

	sarama.Logger = log.New(os.Stdout, "[sarama] ", log.LstdFlags)

	a, err := sarama.NewClusterAdmin(addrs, config)
	if err != nil {
		return nil, err
	}

	return &KafkaIntentsAdmin{kafkaServer: kafkaServer, kafkaAdminClient: a}, nil
}

func (a *KafkaIntentsAdmin) collectTopicsToACLList(principal string, topics []otterizev1alpha1.KafkaTopic) (TopicToACLList, error) {
	topicToACLList := TopicToACLList{}

	for _, topic := range topics {
		operation, ok := KafkaOperationToAclOperationBMap.Get(topic.Operation)
		if !ok {
			return nil, fmt.Errorf("unknown operation %s", topic.Operation)
		}

		acl := sarama.Acl{
			Principal:      principal,
			Host:           "*",
			Operation:      operation,
			PermissionType: sarama.AclPermissionAllow,
		}

		topicToACLList[topic.Name] = append(topicToACLList[topic.Name], &acl)
	}

	return topicToACLList, nil
}

func (a *KafkaIntentsAdmin) deleteACLsByPrincipalTopicsByPrincipal(principal string) (int, error) {
	aclFilter := sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAllow,
		Operation:                 sarama.AclOperationAny,
		Principal:                 lo.ToPtr(principal),
		Host:                      lo.ToPtr("*"),
	}

	matchedAcls, err := a.kafkaAdminClient.DeleteACL(aclFilter, true)
	if err != nil {
		return 0, fmt.Errorf("failed deleting ACLs on server: %w", err)
	}

	return len(matchedAcls), nil
}

func (a *KafkaIntentsAdmin) createACLs(topicToACLList TopicToACLList) error {
	resourceACLs := make([]*sarama.ResourceAcls, 0)
	for topicName, aclList := range topicToACLList {
		resource := sarama.Resource{
			ResourceType:        sarama.AclResourceTopic,
			ResourceName:        topicName,
			ResourcePatternType: sarama.AclPatternLiteral,
		}
		resourceACLs = append(resourceACLs, &sarama.ResourceAcls{Resource: resource, Acls: aclList})
	}

	if err := a.kafkaAdminClient.CreateACLs(resourceACLs); err != nil {
		return fmt.Errorf("failed applying ACLs to server: %w", err)
	}

	return nil
}

func (a *KafkaIntentsAdmin) deleteACLsByPrincipalTopics(principal string, topics []otterizev1alpha1.KafkaTopic) error {
	for _, topic := range topics {
		operation, ok := KafkaOperationToAclOperationBMap.Get(topic.Operation)
		if !ok {
			return fmt.Errorf("unknown operation %s", topic.Operation)
		}
		_, err := a.kafkaAdminClient.DeleteACL(
			sarama.AclFilter{
				ResourceType:              sarama.AclResourceTopic,
				ResourceName:              &topic.Name,
				ResourcePatternTypeFilter: sarama.AclPatternLiteral,
				Principal:                 &principal,
				Operation:                 operation,
				PermissionType:            sarama.AclPermissionAllow,
			}, true)
		if err != nil {
			return fmt.Errorf("failed deleting ACL rules from server: %w", err)
		}
	}

	return nil
}

func (a *KafkaIntentsAdmin) logACLs() error {
	logger := logrus.WithFields(
		logrus.Fields{
			"serverName":      a.kafkaServer.Name,
			"serverNamespace": a.kafkaServer.Namespace,
		})

	aclFilter := sarama.AclFilter{
		ResourceType:              sarama.AclResourceAny,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAny,
		Operation:                 sarama.AclOperationAny,
	}

	acls, err := a.kafkaAdminClient.ListAcls(aclFilter)
	if err != nil {
		return err
	}

	logger.Info("Current state of ACL rules")
	if len(acls) == 0 {
		logger.Info("No ACL rules found")
	}

	for _, aclRules := range acls {
		for _, acl := range aclRules.Acls {
			logger.WithFields(logrus.Fields{
				"ResourceName":   aclRules.Resource.ResourceName,
				"resourceType":   aclRules.Resource.ResourceType,
				"Principal":      acl.Principal,
				"PermissionType": acl.PermissionType,
				"Operation":      acl.Operation,
				"host":           acl.Host,
			}).Info("ACL:")
		}

	}
	return nil
}

func (a *KafkaIntentsAdmin) ApplyIntents(clientName string, clientNamespace string, intents []otterizev1alpha1.Intent) error {
	principal := fmt.Sprintf("User:CN=%s.%s", clientName, clientNamespace)
	logger := logrus.WithFields(
		logrus.Fields{
			"principal":       principal,
			"serverName":      a.kafkaServer.Name,
			"serverNamespace": a.kafkaServer.Namespace,
		})

	appliedKafkaTopics, err := a.getAppliedKafkaTopics(principal)
	if err != nil {
		return fmt.Errorf("failed getting applied ACL rules %w", err)
	}

	newAclRules, AclRulesToDelete := a.kafkaAclDifference(intents, appliedKafkaTopics)

	if len(newAclRules) == 0 {
		logger.Info("No new ACLs found to apply on server")
	} else {
		topicToACLList, err := a.collectTopicsToACLList(principal, newAclRules)
		if err != nil {
			return fmt.Errorf("failed collecting ACLs for server: %w", err)
		}
		logger.Infof("Creating %d new ACLs", len(newAclRules))
		if err := a.createACLs(topicToACLList); err != nil {
			return fmt.Errorf("failed creating ACLs on server: %w", err)
		}
	}

	if len(AclRulesToDelete) == 0 {
		logger.Info("No ACL rules to delete")
	} else {
		logger.Infof("deleting %d ACL rules", len(AclRulesToDelete))
		if err := a.deleteACLsByPrincipalTopics(principal, AclRulesToDelete); err != nil {
			return fmt.Errorf("failed deleting ACLs on server: %w", err)
		}
	}

	if err := a.logACLs(); err != nil {
		logger.WithError(err).Error("failed logging current ACL rules")
	}

	return nil
}

func (a *KafkaIntentsAdmin) RemoveClientIntents(clientName string, clientNamespace string) error {
	principal := fmt.Sprintf("User:CN=%s.%s", clientName, clientNamespace)
	logger := logrus.WithFields(
		logrus.Fields{
			"principal":       principal,
			"serverName":      a.kafkaServer.Name,
			"serverNamespace": a.kafkaServer.Namespace,
		})
	countDeleted, err := a.deleteACLsByPrincipalTopicsByPrincipal(principal)
	if err != nil {
		logger.Errorf("failed clearing acl rules for principal %s", principal)
		return fmt.Errorf("failed clearing acls %w", err)
	}
	logger.Infof("%d acl rules was deleted", countDeleted)

	if err := a.logACLs(); err != nil {
		logger.WithError(err).Error("failed logging current ACL rules")
	}
	return nil
}

func (a *KafkaIntentsAdmin) kafkaAclDifference(intents []otterizev1alpha1.Intent, appliedTopicAcls []otterizev1alpha1.KafkaTopic) ([]otterizev1alpha1.KafkaTopic, []otterizev1alpha1.KafkaTopic) {
	expectedTopicAcls := lo.Flatten(lo.Map(intents, func(intent otterizev1alpha1.Intent, _ int) []otterizev1alpha1.KafkaTopic { return intent.Topics }))
	newAclRules, AclRulesToDelete := lo.Difference(expectedTopicAcls, appliedTopicAcls)
	return newAclRules, AclRulesToDelete
}

func (a *KafkaIntentsAdmin) getAppliedKafkaTopics(principal string) ([]otterizev1alpha1.KafkaTopic, error) {
	appliedKafkaTopics := make([]otterizev1alpha1.KafkaTopic, 0)
	principalAcls, err := a.kafkaAdminClient.ListAcls(sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		Principal:                 &principal,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAllow,
		Operation:                 sarama.AclOperationAny,
	})
	if err != nil {
		return nil, fmt.Errorf("failed listing ACLs on server: %w", err)
	}

	for _, resourceAcls := range principalAcls {
		resourceAppliedKafkaTopics, err := lox.MapErr(resourceAcls.Acls, func(acl *sarama.Acl, _ int) (otterizev1alpha1.KafkaTopic, error) {
			operation, ok := KafkaOperationToAclOperationBMap.GetInverse(acl.Operation)
			if !ok {
				return otterizev1alpha1.KafkaTopic{}, fmt.Errorf("unknown operation %v", acl.Operation)
			}
			return otterizev1alpha1.KafkaTopic{Name: resourceAcls.ResourceName, Operation: operation}, nil
		})
		if err != nil {
			return nil, err
		}

		appliedKafkaTopics = append(appliedKafkaTopics, resourceAppliedKafkaTopics...)
	}

	return appliedKafkaTopics, nil
}

func (a *KafkaIntentsAdmin) RemoveAllIntents() error {
	logger := logrus.WithFields(
		logrus.Fields{
			"serverName":      a.kafkaServer.Name,
			"serverNamespace": a.kafkaServer.Namespace,
		})

	logger.Info("Clearing ACLs from Kafka server")

	aclFilter := sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAllow,
		Operation:                 sarama.AclOperationAny,
	}

	matchedAcls, err := a.kafkaAdminClient.DeleteACL(aclFilter, true)
	if err != nil {
		return fmt.Errorf("failed deleting ACLs on server: %w", err)
	}

	logger.Infof("%d acl rules was deleted", len(matchedAcls))

	return nil
}

func (a *KafkaIntentsAdmin) getExpectedTopicsConfAcls(topicsConf []otterizev1alpha1.TopicConfig) map[sarama.Resource][]sarama.Acl {
	if len(topicsConf) == 0 {
		// default configuration
		topicsConf = []otterizev1alpha1.TopicConfig{
			{Topic: "*", Pattern: "literal", ClientIdentityRequired: true, IntentsRequired: true},
		}
	}

	resourceToAcls := map[sarama.Resource][]sarama.Acl{}
	for _, topicConfig := range topicsConf {
		resource := sarama.Resource{
			ResourceType:        sarama.AclResourceTopic,
			ResourceName:        topicConfig.Topic,
			ResourcePatternType: kafkaPatternTypeToSaramaPatternType[topicConfig.Pattern],
		}

		var acls []sarama.Acl

		// deny/allow ANONYMOUS users any operation to topic according to ClientIdentityRequired config
		acls = append(
			acls,
			sarama.Acl{
				Principal:      AnonymousUserPrincipalName,
				Host:           "*",
				Operation:      sarama.AclOperationAll,
				PermissionType: lo.Ternary(topicConfig.ClientIdentityRequired, sarama.AclPermissionDeny, sarama.AclPermissionAllow),
			},
		)

		if !topicConfig.IntentsRequired {
			// allow ANY user any operation, to implement a default allow policy
			acls = append(
				acls,
				sarama.Acl{
					Principal:      AnyUserPrincipalName,
					Host:           "*",
					Operation:      sarama.AclOperationAll,
					PermissionType: sarama.AclPermissionAllow,
				},
			)
		}

		resourceToAcls[resource] = acls
	}

	return resourceToAcls
}

func (a *KafkaIntentsAdmin) getAppliedTopicsConfAcls() (map[sarama.Resource][]sarama.Acl, error) {
	resourceToAcls := map[sarama.Resource][]sarama.Acl{}
	for _, principal := range []string{AnonymousUserPrincipalName, AnyUserPrincipalName} {
		resourceAclsList, err := a.kafkaAdminClient.ListAcls(sarama.AclFilter{
			ResourceType:              sarama.AclResourceTopic,
			ResourcePatternTypeFilter: sarama.AclPatternAny,
			PermissionType:            sarama.AclPermissionAny,
			Operation:                 sarama.AclOperationAny,
			Principal:                 lo.ToPtr(principal),
		})
		if err != nil {
			return nil, err
		}

		for _, resourceAcls := range resourceAclsList {
			resourceToAcls[resourceAcls.Resource] = append(
				resourceToAcls[resourceAcls.Resource],
				lo.Map(resourceAcls.Acls, func(acl *sarama.Acl, _ int) sarama.Acl {
					return lo.FromPtr(acl)
				})...,
			)
		}
	}

	return resourceToAcls, nil
}

func (a *KafkaIntentsAdmin) kafkaResourceAclsDiff(expected map[sarama.Resource][]sarama.Acl, found map[sarama.Resource][]sarama.Acl) (
	resourceAclsToCreate []*sarama.ResourceAcls, resourceAclsToDelete []*sarama.ResourceAcls) {

	for resource, expectedAcls := range expected {
		existingAcls := found[resource]
		aclsToAdd, aclsToDelete := lo.Difference(expectedAcls, existingAcls)
		if len(aclsToAdd) > 0 {
			resourceAclsToCreate = append(resourceAclsToCreate,
				&sarama.ResourceAcls{
					Resource: resource,
					Acls:     lo.ToSlicePtr(aclsToAdd),
				},
			)
		}
		if len(aclsToDelete) > 0 {
			resourceAclsToDelete = append(resourceAclsToDelete,
				&sarama.ResourceAcls{
					Resource: resource,
					Acls:     lo.ToSlicePtr(aclsToDelete),
				},
			)
		}
	}

	return
}

func (a *KafkaIntentsAdmin) deleteResourceAcls(resourceAclsToDelete []*sarama.ResourceAcls) error {
	for _, resourceAcls := range resourceAclsToDelete {
		for _, acl := range resourceAcls.Acls {
			if _, err := a.kafkaAdminClient.DeleteACL(sarama.AclFilter{
				ResourceType:              resourceAcls.ResourceType,
				ResourceName:              lo.ToPtr(resourceAcls.ResourceName),
				ResourcePatternTypeFilter: resourceAcls.ResourcePatternType,
				PermissionType:            acl.PermissionType,
				Operation:                 acl.Operation,
				Principal:                 lo.ToPtr(acl.Principal),
				Host:                      lo.ToPtr(acl.Host),
			}, false); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *KafkaIntentsAdmin) ApplyServerTopicsConf(topicsConf []otterizev1alpha1.TopicConfig) error {
	logger := logrus.WithFields(
		logrus.Fields{
			"serverName":      a.kafkaServer.Name,
			"serverNamespace": a.kafkaServer.Namespace,
		})

	expectedResourceAcls := a.getExpectedTopicsConfAcls(topicsConf)
	appliedTopicsConfAcls, err := a.getAppliedTopicsConfAcls()
	if err != nil {
		return fmt.Errorf("failed getting applied topic config ACLs: %w", err)
	}

	resourceAclsToCreate, resourceAclsToDelete := a.kafkaResourceAclsDiff(expectedResourceAcls, appliedTopicsConfAcls)

	if len(resourceAclsToCreate) > 0 {
		logger.Infof("Creating %d resource ACLs for topic configurations", len(resourceAclsToCreate))
		for _, resourceAcl := range resourceAclsToCreate {
			for _, acl := range resourceAcl.Acls {
				logger.Infof("Resource: %v, ACL: %v", resourceAcl.Resource, *acl)
			}
		}
		if err := a.kafkaAdminClient.CreateACLs(resourceAclsToCreate); err != nil {
			return fmt.Errorf("failed creating ACLs: %w", err)
		}
	}
	if len(resourceAclsToDelete) > 0 {
		logger.Infof("Delete %d resource ACLs for topic configurations", len(resourceAclsToDelete))
		if err := a.deleteResourceAcls(resourceAclsToDelete); err != nil {
			return fmt.Errorf("failed deleting ACLs: %w", err)
		}
	}

	if err := a.logACLs(); err != nil {
		logger.WithError(err).Error("failed logging current ACL rules")
	}

	return nil
}
