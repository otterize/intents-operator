package intents_reconcilers

//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_k8s_client.go -package=intentsreconcilersmocks sigs.k8s.io/controller-runtime/pkg/client Client
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_istio_manager.go -package=intentsreconcilersmocks -source=../istiopolicy/policy_manager.go PolicyManager
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_service_resolver.go -package=intentsreconcilersmocks -source=../../../shared/serviceidresolver/serviceidresolver.go ServiceResolver
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_external_netpol_handler.go -package=intentsreconcilersmocks -source=./network_policy.go externalNetpolandler
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_policy_agent.go -package=intentsreconcilersmocks -source=./iam_reconciler.go IAMPolicyAgent
