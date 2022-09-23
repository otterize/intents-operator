package kafkaacls

import (
	"errors"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	ServerSpecNotFound = errors.New("failed getting kafka server connection - server configuration specs not set")
)

type ServersStore interface {
	Add(config *otterizev1alpha1.KafkaServerConfig)
	Remove(serverName string, namespace string)
	Exists(serverName string, namespace string) bool
	Get(serverName string, namespace string) (KafkaIntentsAdmin, error)
	MapErr(f func(types.NamespacedName) error) error
}

type serversStore struct {
	serversByName          map[types.NamespacedName]*otterizev1alpha1.KafkaServerConfig
	enableKafkaACLCreation bool
}

func NewServersStore(enableKafkaACLCreation bool) ServersStore {
	return &serversStore{
		serversByName:          map[types.NamespacedName]*otterizev1alpha1.KafkaServerConfig{},
		enableKafkaACLCreation: enableKafkaACLCreation,
	}
}
func (s *serversStore) Add(config *otterizev1alpha1.KafkaServerConfig) {
	name := types.NamespacedName{Name: config.Spec.Service.Name, Namespace: config.Namespace}
	s.serversByName[name] = config
}

func (s *serversStore) Remove(serverName string, namespace string) {
	name := types.NamespacedName{Name: serverName, Namespace: namespace}
	if _, ok := s.serversByName[name]; ok {
		delete(s.serversByName, name)
	}
}

func (s *serversStore) Exists(serverName string, namespace string) bool {
	name := types.NamespacedName{Name: serverName, Namespace: namespace}
	_, ok := s.serversByName[name]
	return ok
}

func (s *serversStore) Get(serverName string, namespace string) (KafkaIntentsAdmin, error) {
	name := types.NamespacedName{Name: serverName, Namespace: namespace}
	config, ok := s.serversByName[name]
	if !ok {
		return nil, ServerSpecNotFound
	}

	return NewKafkaIntentsAdmin(*config, s.enableKafkaACLCreation)
}

func (s *serversStore) MapErr(f func(types.NamespacedName) error) error {
	for serverName, _ := range s.serversByName {
		if err := f(serverName); err != nil {
			return err
		}
	}

	return nil
}
