package serviceidresolver

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=./mocks/mock_k8s_client.go -package=serviceidresolvermocks sigs.k8s.io/controller-runtime/pkg/client Client
