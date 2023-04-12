package istiopolicy

//go:generate curl -sL https://raw.githubusercontent.com/istio/api/master/kubernetes/customresourcedefinitions.gen.yaml -o ./crd/istio-all-crd.yaml
//go:generate go run github.com/golang/mock/mockgen -package istiopolicymocks -destination ./mocks/mock_client.go sigs.k8s.io/controller-runtime/pkg/client Client
