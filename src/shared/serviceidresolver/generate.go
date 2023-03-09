package serviceidresolver

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=./mocks/mock_k8s_client.go -package=serviceidresolvermocks -source=./serviceidresolver.go k8sClient
