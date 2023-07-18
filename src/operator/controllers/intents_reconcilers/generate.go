package intents_reconcilers

//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_k8s_client.go -package=intentsreconcilersmocks sigs.k8s.io/controller-runtime/pkg/client Client
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_endpoints_reconciler.go -package=intentsreconcilersmocks -source=../external_traffic/endpoints_reconciler.go EndpointsReconciler
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_istio_admin.go -package=intentsreconcilersmocks -source=../istiopolicy/admin.go Admin
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_service_resolver.go -package=intentsreconcilersmocks -source=../../../shared/serviceidresolver/serviceidresolver.go ServiceResolver
