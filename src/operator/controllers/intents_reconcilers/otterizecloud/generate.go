package otterizecloud

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=./mocks/mock_cloud_api.go -package=otterizecloudmocks -source=cloud_api.go CloudClient
