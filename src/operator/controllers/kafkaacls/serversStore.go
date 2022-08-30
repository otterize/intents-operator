package kafkaacls

import (
	"errors"
	otterizev1alpha1 "github.com/otterize/intents-operator/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	ServerSpecNotFound = errors.New("failed getting kafka server connection - server configuration specs not set")
)

type ServersStore struct {
	serversByName map[types.NamespacedName]*otterizev1alpha1.KafkaServerConfig
}

func NewServersStore() *ServersStore {
	return &ServersStore{
		serversByName: map[types.NamespacedName]*otterizev1alpha1.KafkaServerConfig{},
	}
}
func (s *ServersStore) Add(config *otterizev1alpha1.KafkaServerConfig) {
	name := types.NamespacedName{Name: config.Spec.ServerName, Namespace: config.Namespace}
	s.serversByName[name] = config
}

func (s *ServersStore) Remove(serverName string, namespace string) {
	name := types.NamespacedName{Name: serverName, Namespace: namespace}
	if _, ok := s.serversByName[name]; ok {
		delete(s.serversByName, name)
	}
}

func (s *ServersStore) Exists(serverName string, namespace string) bool {
	name := types.NamespacedName{Name: serverName, Namespace: namespace}
	_, ok := s.serversByName[name]
	return ok
}

func (s *ServersStore) Get(serverName string, namespace string) (*KafkaIntentsAdmin, error) {
	name := types.NamespacedName{Name: serverName, Namespace: namespace}
	config, ok := s.serversByName[name]
	if !ok {
		return nil, ServerSpecNotFound
	}

	return NewKafkaIntentsAdmin(*config)
}

func (s *ServersStore) MapErr(f func(types.NamespacedName, *KafkaIntentsAdmin) error) error {
	for serverName, config := range s.serversByName {
		a, err := NewKafkaIntentsAdmin(*config)
		if err != nil {
			return err
		}
		if err := f(serverName, a); err != nil {
			return err
		}
	}

	return nil
}
