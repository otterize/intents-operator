package serviceidresolver

//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_k8s_client.go -package=serviceidresolvermocks sigs.k8s.io/controller-runtime/pkg/client Client
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_k8s_restmapper.go -package=serviceidresolvermocks k8s.io/apimachinery/pkg/api/meta RESTMapper
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_service_resolver.go -package=serviceidresolvermocks -source=serviceidresolver.go ServiceResolver
