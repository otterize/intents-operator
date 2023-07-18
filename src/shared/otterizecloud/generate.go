package otterizecloud

//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_cloud_api.go -package=otterizecloudmocks -source=../../operator/controllers/intents_reconcilers/otterizecloud/cloud_api.go CloudClient
