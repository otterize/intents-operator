package kafka_acls

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

type KafkaIntentsAdmin struct {
	kafkaServer      otterizev1alpha1.KafkaServer
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

func NewKafkaIntentsAdmin(kafkaServer otterizev1alpha1.KafkaServer) (*KafkaIntentsAdmin, error) {
	logger := logrus.WithField("addr", kafkaServer.Addr)
	logger.Info("Connecting to kafka server")
	addrs := []string{kafkaServer.Addr}

	config := sarama.NewConfig()
	config.Version = sarama.V2_0_0_0

	tlsConfig, err := getTLSConfig(kafkaServer.TLS)
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

func (a *KafkaIntentsAdmin) collectTopicsToACLList(clientPrincipal string, topics []otterizev1alpha1.KafkaTopic) (TopicToACLList, error) {
	topicToACLList := TopicToACLList{}

	for _, topic := range topics {
		operation, ok := KafkaOperationToAclOperationBMap.Get(topic.Operation)
		if !ok {
			return nil, fmt.Errorf("unknown operation %s", topic.Operation)
		}

		acl := sarama.Acl{
			Principal:      clientPrincipal,
			Host:           "*",
			Operation:      operation,
			PermissionType: sarama.AclPermissionAllow,
		}

		topicToACLList[topic.Name] = append(topicToACLList[topic.Name], &acl)
	}

	return topicToACLList, nil
}

func (a *KafkaIntentsAdmin) clearACLs(clientPrincipal string) error {
	aclFilter := sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAllow,
		Operation:                 sarama.AclOperationAny,
		Principal:                 lo.ToPtr(clientPrincipal),
		Host:                      lo.ToPtr("*"),
	}

	if _, err := a.kafkaAdminClient.DeleteACL(aclFilter, true); err != nil {
		return fmt.Errorf("failed deleting ACLs on server: %w", err)
	}

	return nil
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

func (a *KafkaIntentsAdmin) deleteACLs(principal string, topics []otterizev1alpha1.KafkaTopic) error {
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
		PermissionType:            sarama.AclPermissionAllow,
		Operation:                 sarama.AclOperationAny,
	}

	acls, err := a.kafkaAdminClient.ListAcls(aclFilter)
	if err != nil {
		return err
	}

	logger.Info("Current state of ACL rules")
	for _, aclRules := range acls {
		for _, acl := range aclRules.Acls {
			logger.WithFields(logrus.Fields{
				"ResourceName":   aclRules.Resource.ResourceName,
				"resourceType":   aclRules.Resource.ResourceType,
				"Principal":      acl.Principal,
				"PermissionType": acl.PermissionType,
				"Operation":      acl.Operation,
				"host":           acl.Host,
			}).Infof("ACL:")
		}

	}
	return nil
}

func (a *KafkaIntentsAdmin) ApplyIntents(clientName string, clientNamespace string, intents []otterizev1alpha1.Intent) error {
	clientPrincipal := fmt.Sprintf("User:CN=%s.%s", clientName, clientNamespace)
	logger := logrus.WithFields(
		logrus.Fields{
			"clientPrincipal": clientPrincipal,
			"serverName":      a.kafkaServer.Name,
			"serverNamespace": a.kafkaServer.Namespace,
		})

	appliedKafkaTopics, err := a.getAppliedKafkaTopic(clientPrincipal)
	if err != nil {
		return fmt.Errorf("failed getting applied ACL rules %w", err)
	}
	newAclRules, AclRulesToDelete, err := a.kafkaAclDifference(intents, appliedKafkaTopics)
	if err != nil {
		return err
	}

	topicToACLList, err := a.collectTopicsToACLList(clientPrincipal, newAclRules)
	if err != nil {
		return fmt.Errorf("failed collecting ACLs for server: %w", err)
	}

	if len(topicToACLList) == 0 {
		logger.Info("No new ACLs found to apply on server")
	} else {
		logger.Infof("Creating %d new ACLs", len(newAclRules))
		if err := a.createACLs(topicToACLList); err != nil {
			return fmt.Errorf("failed creating ACLs on server: %w", err)
		}
	}

	if len(AclRulesToDelete) == 0 {
		logger.Info("No ACL rules to delete")
	} else {
		logger.Infof("deleting %d ACL rules", len(AclRulesToDelete))
		if err := a.deleteACLs(clientPrincipal, AclRulesToDelete); err != nil {
			return fmt.Errorf("failed creating ACLs on server: %w", err)
		}
	}

	if err := a.logACLs(); err != nil {
		logger.WithError(err).Error("failed logging current ACL rules")
	}

	return nil
}

func (a *KafkaIntentsAdmin) kafkaAclDifference(intents []otterizev1alpha1.Intent, appliedTopicAcls []otterizev1alpha1.KafkaTopic) ([]otterizev1alpha1.KafkaTopic, []otterizev1alpha1.KafkaTopic, error) {
	kafkaAcls := lo.Flatten(lo.Map(intents, func(intent otterizev1alpha1.Intent, _ int) []otterizev1alpha1.KafkaTopic { return intent.Topics }))
	newAclRules, AclRulesToDelete := lo.Difference(kafkaAcls, appliedTopicAcls)
	return newAclRules, AclRulesToDelete, nil
}

func (a *KafkaIntentsAdmin) getAppliedKafkaTopic(clientPrincipal string) ([]otterizev1alpha1.KafkaTopic, error) {
	appliedKafkaTopics := make([]otterizev1alpha1.KafkaTopic, 0)
	principalAcls, err := a.kafkaAdminClient.ListAcls(sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		Principal:                 &clientPrincipal,
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
