package intents_reconcilers

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=./mocks/mock_k8s_client.go -package=intentsreconcilersmocks sigs.k8s.io/controller-runtime/pkg/client Client
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=./mocks/mock_endpoints_reconciler.go -package=intentsreconcilersmocks -source=../external_traffic/endpoints_reconciler.go EndpointsReconciler
