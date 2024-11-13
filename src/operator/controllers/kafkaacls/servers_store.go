package kafkaacls

import (
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"github.com/otterize/intents-operator/src/shared/errors"
	"k8s.io/apimachinery/pkg/types"
)

var (
	ServerSpecNotFound = errors.NewSentinelError("failed getting kafka server connection - server configuration specs not set")
)

type ServersStore interface {
	Add(config *otterizev2.KafkaServerConfig)
	Remove(serverName string, namespace string)
	Exists(serverName string, namespace string) bool
	Get(serverName string, namespace string) (KafkaIntentsAdmin, error)
	MapErr(f func(types.NamespacedName, *otterizev2.KafkaServerConfig, otterizev2.TLSSource) error) error
}

type ServersStoreImpl struct {
	serversByName               map[types.NamespacedName]*otterizev2.KafkaServerConfig
	enableKafkaACLCreation      bool
	tlsSourceFiles              otterizev2.TLSSource
	IntentsAdminFactoryFunction IntentsAdminFactoryFunction
	enforcementDefaultState     bool
}

func NewServersStore(tlsSourceFiles otterizev2.TLSSource, enableKafkaACLCreation bool, factoryFunc IntentsAdminFactoryFunction, enforcementDefaultState bool) *ServersStoreImpl {
	return &ServersStoreImpl{
		serversByName:               map[types.NamespacedName]*otterizev2.KafkaServerConfig{},
		enableKafkaACLCreation:      enableKafkaACLCreation,
		tlsSourceFiles:              tlsSourceFiles,
		IntentsAdminFactoryFunction: factoryFunc,
		enforcementDefaultState:     enforcementDefaultState,
	}
}

func (s *ServersStoreImpl) Add(config *otterizev2.KafkaServerConfig) {
	name := types.NamespacedName{Name: config.Spec.Service.Name, Namespace: config.Namespace}
	s.serversByName[name] = config
}

func (s *ServersStoreImpl) Remove(serverName string, namespace string) {
	name := types.NamespacedName{Name: serverName, Namespace: namespace}
	delete(s.serversByName, name)
}

func (s *ServersStoreImpl) Exists(serverName string, namespace string) bool {
	name := types.NamespacedName{Name: serverName, Namespace: namespace}
	_, ok := s.serversByName[name]
	return ok
}

func (s *ServersStoreImpl) Get(serverName string, namespace string) (KafkaIntentsAdmin, error) {
	name := types.NamespacedName{Name: serverName, Namespace: namespace}
	config, ok := s.serversByName[name]
	if !ok {
		return nil, ServerSpecNotFound
	}

	return s.IntentsAdminFactoryFunction(*config, s.tlsSourceFiles, s.enableKafkaACLCreation, s.enforcementDefaultState)
}

func (s *ServersStoreImpl) MapErr(f func(types.NamespacedName, *otterizev2.KafkaServerConfig, otterizev2.TLSSource) error) error {
	for serverName, config := range s.serversByName {
		if err := f(serverName, config, s.tlsSourceFiles); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}
