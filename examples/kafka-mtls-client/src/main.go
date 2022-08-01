package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/Shopify/sarama"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"log"
	"os"
)

const (
	kafkaAddr  = "kafka-tls-0.kafka-tls-headless.default.svc.cluster.local:9092"
	certFile   = "/etc/spire-integration/svid.pem"
	keyFile    = "/etc/spire-integration/key.pem"
	rootCAFile = "/etc/spire-integration/bundle.pem"
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

func main() {
	logrus.WithField("addr", kafkaAddr).Info("Connecting to kafka server")
	addrs := []string{kafkaAddr}

	config := sarama.NewConfig()
	config.Version = sarama.V2_0_0_0

	tlsConfig, err := getTLSConfig()
	if err != nil {
		logrus.WithError(err).Panic()
	}

	config.Net.TLS.Config = tlsConfig
	config.Net.TLS.Enable = true

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

}
