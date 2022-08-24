package kafkaacls

import (
	"github.com/otterize/intents-operator/shared/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

type ServersStore struct {
	serversByName map[types.NamespacedName]*KafkaIntentsAdmin
}

func NewServersStore() *ServersStore {
	return &ServersStore{
		serversByName: map[types.NamespacedName]*KafkaIntentsAdmin{},
	}
}
func (s *ServersStore) Add(config *v1alpha1.KafkaServerConfig) (*KafkaIntentsAdmin, error) {
	a, err := NewKafkaIntentsAdmin(*config)
	if err != nil {
		return nil, err
	}

	name := types.NamespacedName{Name: config.Spec.ServerName, Namespace: config.Namespace}
	s.serversByName[name] = a
	return a, nil
}

func (s *ServersStore) Remove(serverName string, namespace string) {
	name := types.NamespacedName{Name: serverName, Namespace: namespace}
	if _, ok := s.serversByName[name]; ok {
		delete(s.serversByName, name)
	}
}

func (s *ServersStore) Get(serverName string, namespace string) (*KafkaIntentsAdmin, bool) {
	name := types.NamespacedName{Name: serverName, Namespace: namespace}
	a, ok := s.serversByName[name]
	return a, ok
}

func (s *ServersStore) MapErr(f func(types.NamespacedName, *KafkaIntentsAdmin) error) error {
	for serverName, a := range s.serversByName {
		if err := f(serverName, a); err != nil {
			return err
		}
	}

	return nil
}
