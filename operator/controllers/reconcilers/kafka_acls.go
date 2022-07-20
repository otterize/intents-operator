package reconcilers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/Shopify/sarama"
	otterizev1alpha1 "github.com/otterize/otternose/api/v1alpha1"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"log"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KafkaACLsReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	kafkaServer      otterizev1alpha1.KafkaServer
	kafkaAdminClient sarama.ClusterAdmin
}

func getTLSConfig(tlsSource otterizev1alpha1.TLSSource) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(tlsSource.CertFile, tlsSource.KeyFile)
	if err != nil {
		return nil, err
	}

	pool := x509.NewCertPool()
	bundle, err := ioutil.ReadFile(tlsSource.RootCAFile)
	if err != nil {
		return nil, err
	}
	pool.AppendCertsFromPEM(bundle)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	}, nil
}

func getKafkaAdminClient(server otterizev1alpha1.KafkaServer) (sarama.ClusterAdmin, error) {
	logrus.WithField("addr", server.Addr).Info("Connecting to kafka server")
	addrs := []string{server.Addr}

	config := sarama.NewConfig()
	config.Version = sarama.V2_0_0_0

	tlsConfig, err := getTLSConfig(server.TLS)
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

	topics, err := a.ListTopics()
	if err != nil {
		return nil, err
	}

	logrus.Infof("Topics: %v", topics)
	return a, nil
}

func NewKafkaACLsReconciler(client client.Client, scheme *runtime.Scheme, kafkaServer otterizev1alpha1.KafkaServer) (*KafkaACLsReconciler, error) {
	r := KafkaACLsReconciler{Client: client, Scheme: scheme, kafkaServer: kafkaServer}
	kafkaAdminClient, err := getKafkaAdminClient(kafkaServer)
	if err != nil {
		return nil, err
	}
	r.kafkaAdminClient = kafkaAdminClient
	return &r, nil
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

func (r *KafkaACLsReconciler) clearACLs(principal string) error {
	aclFilter := sarama.AclFilter{
		ResourceType:              sarama.AclResourceTopic,
		ResourcePatternTypeFilter: sarama.AclPatternAny,
		PermissionType:            sarama.AclPermissionAllow,
		Operation:                 sarama.AclOperationAny,
		Principal:                 lo.ToPtr(principal),
		Host:                      lo.ToPtr("*"),
	}
	if _, err := r.kafkaAdminClient.DeleteACL(aclFilter, true); err != nil {
		return err
	}

	return nil
}

func (r *KafkaACLsReconciler) createACLs(principal string, intentsNamespace string, calls []otterizev1alpha1.Intent) error {
	topicToAclList := map[string][]*sarama.Acl{}

	for _, call := range calls {
		if call.Type != otterizev1alpha1.IntentTypeKafka || call.Topics == nil {
			// not for kafka
			continue
		}

		serverNamespace := lo.Ternary(call.Namespace != "", call.Namespace, intentsNamespace)

		if serverNamespace != r.kafkaServer.Namespace || call.Server != r.kafkaServer.Name {
			// not intended for this kafka host
			continue
		}

		for _, topic := range call.Topics {
			operation, err := intentOperationToACLOperation(topic.Operation)
			if err != nil {
				return err
			}
			acl := sarama.Acl{
				Principal:      principal,
				Host:           "*",
				Operation:      operation,
				PermissionType: sarama.AclPermissionAllow,
			}
			topicToAclList[topic.Name] = append(topicToAclList[topic.Name], &acl)
		}
	}

	resourceACLs := make([]*sarama.ResourceAcls, 0)

	for topicName, aclList := range topicToAclList {
		resource := sarama.Resource{
			ResourceType:        sarama.AclResourceTopic,
			ResourceName:        topicName,
			ResourcePatternType: sarama.AclPatternLiteral,
		}
		resourceACLs = append(resourceACLs, &sarama.ResourceAcls{Resource: resource, Acls: aclList})
	}

	if len(resourceACLs) == 0 {
		return nil
	}

	if err := r.kafkaAdminClient.CreateACLs(resourceACLs); err != nil {
		return err
	}

	return nil
}

func (r *KafkaACLsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha1.Intents{}
	logger := logrus.WithField("namespaced_name", req.NamespacedName.String())
	err := r.Get(ctx, req.NamespacedName, intents)
	if err != nil && k8serrors.IsNotFound(err) {
		logger.Info("No intents found")
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if intents.Spec == nil {
		logger.Info("No specs found")
		return ctrl.Result{}, nil
	}

	principal := fmt.Sprintf("User:CN=%s.%s", intents.Spec.Service.Name, intents.Namespace)

	// TODO: should actually diff and delete only removed ACLs
	logger = logger.WithField("principal", principal)
	logger.Info("Clearing old ACLs")
	if err := r.clearACLs(principal); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Creating new ACLs")
	if err := r.createACLs(principal, intents.Namespace, intents.Spec.Service.Calls); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil

}
