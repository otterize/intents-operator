package kafkaacls

//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_cluster_admin.go -package=kafkaaclsmocks github.com/Shopify/sarama ClusterAdmin
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_intents_admin.go -package=kafkaaclsmocks -source=intents_admin.go ClusterAdmin
