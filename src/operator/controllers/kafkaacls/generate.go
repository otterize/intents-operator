package kafkaacls

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=./mocks/mock_cluster_admin.go -package=kafkaaclsmocks github.com/Shopify/sarama ClusterAdmin
