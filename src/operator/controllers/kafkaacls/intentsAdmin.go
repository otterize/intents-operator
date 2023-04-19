package kafkaacls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/Shopify/sarama"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/otterize/lox"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/vishalkuo/bimap"
	"log"
	"os"
	"regexp"
	"strings"
)

type IntentsAdminFactoryFunction func(serverConfig otterizev1alpha2.KafkaServerConfig, _ otterizev1alpha2.TLSSource, enableKafkaACLCreation bool, enforcementEnabledGlobally bool) (KafkaIntentsAdmin, error)

type TopicToACLList map[sarama.Resource][]sarama.Acl

const (
	AnonymousUserPrincipalName = "User:ANONYMOUS"
	AnyUserPrincipalName       = "User:*"
)

var (
	serviceNameRE = regexp.MustCompile(`\$ServiceName`)
	namespaceRE   = regexp.MustCompile(`\$Namespace`)
)

type KafkaIntentsAdmin interface {
	ApplyServerTopicsConf(topicsConf []otterizev1alpha2.TopicConfig) error
	ApplyClientIntents(clientName string, clientNamespace string, intents []otterizev1alpha2.Intent) error
	RemoveClientIntents(clientName string, clientNamespace string) error
	RemoveAllIntents() error
	Close()
}

type KafkaIntentsAdminImpl struct {
	kafkaServer                otterizev1alpha2.KafkaServerConfig
	kafkaAdminClient           sarama.ClusterAdmin
	userNameMapping            string
	enableKafkaACLCreation     bool
	enforcementEnabledGlobally bool
}

var (
	kafkaOperationToAclOperation = map[otterizev1alpha2.KafkaOperation]sarama.AclOperation{
		otterizev1alpha2.KafkaOperationAll:             sarama.AclOperationAll,
		otterizev1alpha2.KafkaOperationConsume:         sarama.AclOperationRead,
		otterizev1alpha2.KafkaOperationProduce:         sarama.AclOperationWrite,
		otterizev1alpha2.KafkaOperationCreate:          sarama.AclOperationCreate,
		otterizev1alpha2.KafkaOperationDelete:          sarama.AclOperationDelete,
		otterizev1alpha2.KafkaOperationAlter:           sarama.AclOperationAlter,
		otterizev1alpha2.KafkaOperationDescribe:        sarama.AclOperationDescribe,
		otterizev1alpha2.KafkaOperationClusterAction:   sarama.AclOperationClusterAction,
		otterizev1alpha2.KafkaOperationDescribeConfigs: sarama.AclOperationDescribeConfigs,
		otterizev1alpha2.KafkaOperationAlterConfigs:    sarama.AclOperationAlterConfigs,
		otterizev1alpha2.KafkaOperationIdempotentWrite: sarama.AclOperationIdempotentWrite,
	}
	KafkaOperationToAclOperationBMap = bimap.NewBiMapFromMap(kafkaOperationToAclOperation)

	kafkaPatternTypeToSaramaPatternType = map[otterizev1alpha2.ResourcePatternType]sarama.AclResourcePatternType{
		otterizev1alpha2.ResourcePatternTypeLiteral: sarama.AclPatternLiteral,
		otterizev1alpha2.ResourcePatternTypePrefix:  sarama.AclPatternPrefixed,
	}
)

func getTLSConfig(tlsSource otterizev1alpha2.TLSSource) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(tlsSource.CertFile, tlsSource.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed loading x509 key pair: %w", err)
	}

	pool := x509.NewCertPool()
	rootCAPEM, err := os.ReadFile(tlsSource.RootCAFile)
	if err != nil {
		return nil, fmt.Errorf("failed loading root CA PEM file: %w ", err)
	}
	pool.AppendCertsFromPEM(rootCAPEM)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	}, nil
}

func getUserPrincipalMapping(tlsCert tls.Certificate) (string, error) {
	parsedCert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return "", fmt.Errorf("failed parsing certificate: %w", err)
	}
	// as mentioned in the documentation, SSL user name will be of the form:
	// "CN=writeuser,OU=Unknown,O=Unknown,L=Unknown,ST=Unknown,C=Unknown"  (order sensitive)
	// https://kafka.apache.org/documentation/#security_authz_ssl:~:text=Customizing%20SSL%20User%20Name
	subjectParts := []string{"CN=$ServiceName.$Namespace"}
	if len(parsedCert.Subject.OrganizationalUnit) > 0 {
		subjectParts = append(subjectParts, "OU="+parsedCert.Subject.OrganizationalUnit[0])
	}
	if len(parsedCert.Subject.Organization) > 0 {
		subjectParts = append(subjectParts, "O="+parsedCert.Subject.Organization[0])
	}

	if len(parsedCert.Subject.Locality) > 0 {
		subjectParts = append(subjectParts, "L="+parsedCert.Subject.Locality[0])
	}

	if len(parsedCert.Subject.Province) > 0 {
		// this is not a mistake ST stands for stateOrProvince
		subjectParts = append(subjectParts, "ST="+parsedCert.Subject.Province[0])
	}

	if len(parsedCert.Subject.Country) > 0 {
		subjectParts = append(subjectParts, "C="+parsedCert.Subject.Country[0])
	}

	return strings.Join(subjectParts, ","), nil

}

func NewKafkaIntentsAdmin(kafkaServer otterizev1alpha2.KafkaServerConfig, defaultTls otterizev1alpha2.TLSSource, enableKafkaACLCreation bool, enforcementEnabledGlobally bool) (KafkaIntentsAdmin, error) {
	logger := logrus.WithField("addr", kafkaServer.Spec.Addr)
	logger.Info("Connecting to kafka server")
	addrs := []string{kafkaServer.Spec.Addr}

	config := sarama.NewConfig()
	config.Version = sarama.V2_0_0_0

	var tlsSource otterizev1alpha2.TLSSource
	if lo.IsEmpty(kafkaServer.Spec.TLS) {
		tlsSource = defaultTls
		logger.Info("Using TLS configuration from default")
	} else {
		tlsSource = kafkaServer.Spec.TLS
		logger.Info("Using TLS configuration from KafkaServerConfig")
	}

	tlsConfig, err := getTLSConfig(tlsSource)
	if err != nil {
		return nil, err
	}

	usernameMapping, err := getUserPrincipalMapping(tlsConfig.Certificates[0])
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

	return NewKafkaIntentsAdminImpl(kafkaServer, a, usernameMapping, enableKafkaACLCreation, enforcementEnabledGlobally), nil
}

func NewKafkaIntentsAdminImpl(kafkaServer otterizev1alpha2.KafkaServerConfig, a sarama.ClusterAdmin, usernameMapping string, enableKafkaACLCreation bool, enforcementEnabledGlobally bool) KafkaIntentsAdmin {
	return &KafkaIntentsAdminImpl{kafkaServer: kafkaServer, kafkaAdminClient: a, userNameMapping: usernameMapping, enableKafkaACLCreation: enableKafkaACLCreation, enforcementEnabledGlobally: enforcementEnabledGlobally}
}

func (a *KafkaIntentsAdminImpl) Close() {
	if err := a.kafkaAdminClient.Close(); err != nil {
		logrus.WithError(err).Error("Error closing kafka admin client")
	}
}

func (a *KafkaIntentsAdminImpl) formatPrincipal(clientName string, clientNamespace string) string {
	username := a.userNameMapping
	username = serviceNameRE.ReplaceAllString(username, clientName)
	username = namespaceRE.ReplaceAllString(username, clientNamespace)
	return fmt.Sprintf("User:%s", username)
}

func (a *KafkaIntentsAdminImpl) queryAppliedIntentKafkaTopics(principal string) ([]otterizev1alpha2.KafkaTopic, error) {
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

	resourceAppliedKafkaTopics, err := lox.MapErr(principalAcls, func(acls sarama.ResourceAcls, _ int) (otterizev1alpha2.KafkaTopic, error) {
		operations := make([]otterizev1alpha2.KafkaOperation, 0)
		for _, acl := range acls.Acls {
			operation, ok := KafkaOperationToAclOperationBMap.GetInverse(acl.Operation)
			if !ok {
				return otterizev1alpha2.KafkaTopic{}, fmt.Errorf("unknown operation %v", acl.Operation)
			}
			operations = append(operations, operation)
		}
		return otterizev1alpha2.KafkaTopic{Name: acls.ResourceName, Operations: operations}, nil
	})

	if err != nil {
		return nil, err
	}

	return resourceAppliedKafkaTopics, nil
}

func (a *KafkaIntentsAdminImpl) collectTopicsToACLList(principal string, topics []otterizev1alpha2.KafkaTopic) (TopicToACLList, error) {
	topicToACLList := TopicToACLList{}

	for _, topic := range topics {
		resource := sarama.Resource{
			ResourceType:        sarama.AclResourceTopic,
			ResourceName:        topic.Name,
			ResourcePatternType: sarama.AclPatternLiteral,
		}
		acls := make([]sarama.Acl, 0)
		for _, operation := range topic.Operations {
			operation, ok := KafkaOperationToAclOperationBMap.Get(operation)
			if !ok {
				return nil, fmt.Errorf("unknown operation '%v'", operation)
			}

			acl := sarama.Acl{
				Principal:      principal,
				Host:           "*",
				Operation:      operation,
				PermissionType: sarama.AclPermissionAllow,
			}
			acls = append(acls, acl)
		}
		topicToACLList[resource] = acls
	}

	return topicToACLList, nil
}

func (a *KafkaIntentsAdminImpl) deleteACLsByPrincipal(principal string) (int, error) {
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

func (a *KafkaIntentsAdminImpl) logACLs() error {
	logger := logrus.WithFields(
		logrus.Fields{
			"serverName":      a.kafkaServer.Spec.Service,
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
				"ResourceName":        aclRules.Resource.ResourceName,
				"ResourcePatternType": aclRules.Resource.ResourcePatternType.String(),
				"ResourceType":        aclRules.Resource.ResourceType.String(),
				"Principal":           acl.Principal,
				"PermissionType":      acl.PermissionType.String(),
				"Operation":           acl.Operation.String(),
				"Host":                acl.Host,
			}).Info("ACL:")
		}

	}
	return nil
}

func (a *KafkaIntentsAdminImpl) ApplyClientIntents(clientName string, clientNamespace string, intents []otterizev1alpha2.Intent) error {
	principal := a.formatPrincipal(clientName, clientNamespace)
	logger := logrus.WithFields(
		logrus.Fields{
			"principal":       principal,
			"serverName":      a.kafkaServer.Spec.Service,
			"serverNamespace": a.kafkaServer.Namespace,
		})

	appliedIntentKafkaTopics, err := a.queryAppliedIntentKafkaTopics(principal)
	if err != nil {
		return fmt.Errorf("failed getting applied ACL rules %w", err)
	}

	appliedIntentKafkaAcls, err := a.collectTopicsToACLList(principal, appliedIntentKafkaTopics)
	if err != nil {
		return fmt.Errorf("failed collecting topics to ACL list %w", err)
	}

	expectedIntentKafkaTopics := lo.Flatten(
		lo.Map(intents, func(intent otterizev1alpha2.Intent, _ int) []otterizev1alpha2.KafkaTopic {
			return intent.Topics
		}),
	)
	expectedIntentsKafkaTopicsAcls, err := a.collectTopicsToACLList(principal, expectedIntentKafkaTopics)
	if err != nil {
		return fmt.Errorf("failed collecting topics to ACL list %w", err)
	}

	resourceAclsCreate, resourceAclsDelete := a.kafkaResourceAclsDiff(expectedIntentsKafkaTopicsAcls, appliedIntentKafkaAcls)

	if len(resourceAclsCreate) == 0 {
		logger.Info("No new ACLs found to apply on server")
	} else {
		if a.enforcementEnabledGlobally && a.enableKafkaACLCreation {
			logger.Infof("Creating %d new ACLs", len(resourceAclsCreate))
			if err := a.kafkaAdminClient.CreateACLs(resourceAclsCreate); err != nil {
				return fmt.Errorf("failed applying ACLs to server: %w", err)
			}
			telemetrysender.Send(telemetriesgql.EventTypeKafkaAclCreated, len(resourceAclsCreate))
		} else if !a.enableKafkaACLCreation {
			logger.Infof("Skipped creation of %d new ACLs because Kafka ACL Creation is disabled", len(resourceAclsCreate))
		} else if !a.enforcementEnabledGlobally {
			logger.Infof("Skipped creation of %d new enforcement is globally disabled", len(resourceAclsCreate))
		}
	}

	if len(resourceAclsDelete) == 0 {
		logger.Info("No ACL rules to delete")
	} else {
		logger.Infof("deleting %d ACL rules", len(resourceAclsDelete))
		if err := a.deleteResourceAcls(resourceAclsDelete); err != nil {
			return fmt.Errorf("failed deleting ACLs on server: %w", err)
		}
		telemetrysender.Send(telemetriesgql.EventTypeKafkaAclDeleted, len(resourceAclsCreate))
	}

	if err := a.logACLs(); err != nil {
		logger.WithError(err).Error("failed logging current ACL rules")
	}
	return nil
}

func (a *KafkaIntentsAdminImpl) RemoveClientIntents(clientName string, clientNamespace string) error {
	principal := a.formatPrincipal(clientName, clientNamespace)
	logger := logrus.WithFields(
		logrus.Fields{
			"principal":       principal,
			"serverName":      a.kafkaServer.Spec.Service,
			"serverNamespace": a.kafkaServer.Namespace,
		})
	countDeleted, err := a.deleteACLsByPrincipal(principal)
	if err != nil {
		return fmt.Errorf("failed clearing acls for principal %s: %w", principal, err)
	}
	logger.Infof("%d acl rules was deleted", countDeleted)

	if err := a.logACLs(); err != nil {
		logger.WithError(err).Error("failed logging current ACL rules")
	}
	return nil
}

func (a *KafkaIntentsAdminImpl) RemoveAllIntents() error {
	logger := logrus.WithFields(
		logrus.Fields{
			"serverName":      a.kafkaServer.Spec.Service,
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

	logger.Infof("%d topic acl rules were deleted", len(matchedAcls))

	deletedRulesCount, err := a.deleteConsumerGroupWildcardACLs()
	if err != nil {
		return fmt.Errorf("failed deleting consumer group ACLs on server: %w", err)
	}
	logger.Infof("%d group acl rules were deleted", deletedRulesCount)

	return nil
}

func (a *KafkaIntentsAdminImpl) getExpectedTopicsConfAcls(topicsConf []otterizev1alpha2.TopicConfig) map[sarama.Resource][]sarama.Acl {
	if len(topicsConf) == 0 {
		// default configuration
		topicsConf = []otterizev1alpha2.TopicConfig{
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

func (a *KafkaIntentsAdminImpl) getAppliedTopicsConfAcls() (map[sarama.Resource][]sarama.Acl, error) {
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

func (a *KafkaIntentsAdminImpl) kafkaResourceAclsDiff(expected map[sarama.Resource][]sarama.Acl, found map[sarama.Resource][]sarama.Acl) (
	resourceAclsToCreate []*sarama.ResourceAcls, resourceAclsToDelete []*sarama.ResourceAcls) {

	// handle added / updated resources
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

	// handle deleted resources
	for resource, existingAcls := range found {
		if _, ok := expected[resource]; !ok {
			resourceAclsToDelete = append(resourceAclsToDelete,
				&sarama.ResourceAcls{
					Resource: resource,
					Acls:     lo.ToSlicePtr(existingAcls),
				},
			)
		}
	}

	return resourceAclsToCreate, resourceAclsToDelete
}

func (a *KafkaIntentsAdminImpl) deleteResourceAcls(resourceAclsToDelete []*sarama.ResourceAcls) error {
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

func (a *KafkaIntentsAdminImpl) ApplyServerTopicsConf(topicsConf []otterizev1alpha2.TopicConfig) error {
	logger := logrus.WithFields(
		logrus.Fields{
			"serverName":      a.kafkaServer.Spec.Service,
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
	} else {
		logger.Info("No new ACLs to create for topic configuration")
	}

	if len(resourceAclsToDelete) > 0 {
		logger.Infof("Delete %d resource ACLs for topic configurations", len(resourceAclsToDelete))
		if err := a.deleteResourceAcls(resourceAclsToDelete); err != nil {
			return fmt.Errorf("failed deleting ACLs: %w", err)
		}
	} else {
		logger.Info("No existing ACLs to delete for topic configuration")
	}

	logger.Infof("ensuring consumer group permissions")
	if err := a.ensureConsumerGroupWildcardACLs(); err != nil {
		logger.WithError(err).Error("failed ensuring Consumer group permissions")
	}

	if err := a.logACLs(); err != nil {
		logger.WithError(err).Error("failed logging current ACL rules")
	}

	return nil
}

func (a *KafkaIntentsAdminImpl) ensureConsumerGroupWildcardACLs() error {
	// in order to use a consumer group, a consumer needs read and describe privileges on that group.
	// although read and describe privileges on consumer group resource are required for fetching and committing offsets
	// further topic level privileges should be granted as well: https://kafka.apache.org/documentation/#operations_resources_and_protocols
	r := sarama.Resource{ResourceType: sarama.AclResourceGroup, ResourceName: "*", ResourcePatternType: sarama.AclPatternLiteral}
	groupDescribeACL := &sarama.Acl{Principal: "User:*", Operation: sarama.AclOperationDescribe, PermissionType: sarama.AclPermissionAllow, Host: "*"}
	groupReadACL := &sarama.Acl{Principal: "User:*", Operation: sarama.AclOperationRead, PermissionType: sarama.AclPermissionAllow, Host: "*"}
	// if exists no error will be thrown
	err := a.kafkaAdminClient.CreateACLs([]*sarama.ResourceAcls{{Resource: r, Acls: []*sarama.Acl{groupDescribeACL, groupReadACL}}})
	if err != nil {
		return err
	}
	return nil
}

func (a *KafkaIntentsAdminImpl) deleteConsumerGroupWildcardACLs() (int, error) {
	matchingAclRules, err := a.kafkaAdminClient.DeleteACL(
		sarama.AclFilter{
			ResourceType:              sarama.AclResourceGroup,
			ResourceName:              lo.ToPtr("*"),
			ResourcePatternTypeFilter: sarama.AclPatternLiteral,
			PermissionType:            sarama.AclPermissionAllow,
			Principal:                 lo.ToPtr(AnyUserPrincipalName),
			Operation:                 sarama.AclOperationAny,
		}, false)
	if err != nil {
		return 0, err
	}
	return len(matchingAclRules), nil
}
