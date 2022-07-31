package kafka_acls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/Shopify/sarama"
	otterizev1alpha1 "github.com/otterize/intents-operator/shared/api/v1alpha1"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"log"
	"os"
)

type TopicToACLList map[string][]*sarama.Acl

type KafkaIntentsAdmin struct {
	kafkaServer      otterizev1alpha1.KafkaServer
	kafkaAdminClient sarama.ClusterAdmin
}

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

func intentOperationToACLOperation(operation otterizev1alpha1.KafkaOperation) (sarama.AclOperation, error) {
	switch operation {
	case otterizev1alpha1.KafkaOperationConsume:
		return sarama.AclOperationRead, nil
	case otterizev1alpha1.KafkaOperationProduce:
		return sarama.AclOperationWrite, nil
	case otterizev1alpha1.KafkaOperationCreate:
		return sarama.AclOperationCreate, nil
	case otterizev1alpha1.KafkaOperationDelete:
		return sarama.AclOperationDelete, nil
	case otterizev1alpha1.KafkaOperationAlter:
		return sarama.AclOperationAlter, nil
	case otterizev1alpha1.KafkaOperationDescribe:
		return sarama.AclOperationDescribe, nil
	case otterizev1alpha1.KafkaOperationClusterAction:
		return sarama.AclOperationClusterAction, nil
	case otterizev1alpha1.KafkaOperationDescribeConfigs:
		return sarama.AclOperationDescribeConfigs, nil
	case otterizev1alpha1.KafkaOperationAlterConfigs:
		return sarama.AclOperationAlterConfigs, nil
	case otterizev1alpha1.KafkaOperationIdempotentWrite:
		return sarama.AclOperationIdempotentWrite, nil
	default:
		return sarama.AclOperationUnknown, fmt.Errorf("unknown operation %s", operation)
	}
}

func (a *KafkaIntentsAdmin) collectTopicsToACLList(clientPrincipal string, intents []otterizev1alpha1.Intent) (TopicToACLList, error) {
	topicToACLList := TopicToACLList{}

	for _, intent := range intents {
		for _, topic := range intent.Topics {
			operation, err := intentOperationToACLOperation(topic.Operation)
			if err != nil {
				return nil, err
			}

			acl := sarama.Acl{
				Principal:      clientPrincipal,
				Host:           "*",
				Operation:      operation,
				PermissionType: sarama.AclPermissionAllow,
			}

			topicToACLList[topic.Name] = append(topicToACLList[topic.Name], &acl)
		}
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
	topicToACLList, err := a.collectTopicsToACLList(clientPrincipal, intents)
	if err != nil {
		return fmt.Errorf("failed collecting ACLs for server: %w", err)
	}

	// TODO: should actually diff and delete only removed ACLs
	logger.Info("Clearing old ACLs")
	if err := a.clearACLs(clientPrincipal); err != nil {
		return fmt.Errorf("failed clearing ACLs on server: %w", err)
	}

	if len(topicToACLList) == 0 {
		logger.Info("No new ACLs found to apply on server")
	} else {
		logger.Info("Creating new ACLs")
		if err := a.createACLs(topicToACLList); err != nil {
			return fmt.Errorf("failed creating ACLs on server: %w", err)
		}
	}

	if err := a.logACLs(); err != nil {
		logger.WithError(err).Error("failed logging current ACL rules")
	}

	return nil
}
