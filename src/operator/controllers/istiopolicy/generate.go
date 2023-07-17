package istiopolicy

//go:generate go run github.com/golang/mock/mockgen -package istiopolicymocks -destination ./mocks/mock_client.go sigs.k8s.io/controller-runtime/pkg/client Client
