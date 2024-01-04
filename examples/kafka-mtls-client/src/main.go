package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/Shopify/sarama"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
)

const (
	kafkaAddr  = "kafka-0.kafka-headless.default.svc.cluster.local:9092"
	tlsEnabled = true
	//kafkaAddr     = "kafka-tls2.default.svc.cluster.local:9095"
	//tlsEnabled    = false
	testTopicName = "orders"
	certFile      = "/etc/spire-integration/cert.pem"
	keyFile       = "/etc/spire-integration/key.pem"
	rootCAFile    = "/etc/spire-integration/ca.pem"
)

func getTLSConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed loading x509 key pair: %w", err)
	}

	pool := x509.NewCertPool()
	rootCAPEM, err := ioutil.ReadFile(rootCAFile)
	if err != nil {
		return nil, fmt.Errorf("failed loading root CA PEM file: %w ", err)
	}
	pool.AppendCertsFromPEM(rootCAPEM)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	}, nil
}

func ensureKafkaTopic(client sarama.Client, topic string) error {
	logrus.WithField("topicName", testTopicName).Info("Ensuring topic")

	admin, err := sarama.NewClusterAdminFromClient(client)
	if err != nil {
		return errors.Wrap(err)
	}

	topics, err := admin.ListTopics()
	if err != nil {
		return fmt.Errorf("failed listing topics: %w", err)
	}

	if _, found := topics[topic]; !found {
		err = admin.CreateTopic(topic, &sarama.TopicDetail{
			NumPartitions:     1,
			ReplicationFactor: 1,
		}, false)
		if err != nil {
			return fmt.Errorf("failed creating topic: %w", err)
		}
	}

	topicsMeta, err := admin.DescribeTopics([]string{topic})
	if err != nil {
		return fmt.Errorf("failed describing topic: %w", err)
	}

	for _, topicMeta := range topicsMeta {
		logrus.WithFields(logrus.Fields{"topicMeta": topicMeta, "topic": topic}).Info("Topic metadata")
	}

	return nil
}

func main() {
	logrus.WithField("addr", kafkaAddr).Info("Connecting to kafka server")
	addrs := []string{kafkaAddr}

	config := sarama.NewConfig()
	config.Version = sarama.V2_0_0_0

	config.Net.TLS.Enable = tlsEnabled
	if tlsEnabled {
		tlsConfig, err := getTLSConfig()
		if err != nil {
			logrus.WithError(err).Panic()
		}
		config.Net.TLS.Config = tlsConfig
	}

	sarama.Logger = log.New(os.Stdout, "[sarama] ", log.LstdFlags)

	a, err := sarama.NewClient(addrs, config)
	if err != nil {
		logrus.WithError(err).Panic()
	}

	topics, err := a.Topics()
	if err != nil {
		logrus.WithError(err).Panic()
	}

	logrus.WithField("topics", topics).Info("Topics")

	if err := ensureKafkaTopic(a, testTopicName); err != nil {
		logrus.WithError(err).Panic()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-ctx.Done()

}
