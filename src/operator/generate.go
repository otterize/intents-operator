package main

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/bundlev1/mock.go github.com/spiffe/spire-api-sdk/proto/spire/api/server/bundle/v1 BundleClient
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/svidv1/mock.go github.com/spiffe/spire-api-sdk/proto/spire/api/server/svid/v1 SVIDClient
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/entryv1/mock.go github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1 EntryClient
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/controller-runtime/client/mock.go sigs.k8s.io/controller-runtime/pkg/client Client,Reader,Writer
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/spireclient/mock.go github.com/otterize/spire-integration-operator/src/controllers/spireclient ServerClient
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/spireclient/bundles/mock.go github.com/otterize/spire-integration-operator/src/controllers/spireclient/bundles Store
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -package mock_entries -destination=mocks/entries/mock.go github.com/otterize/spire-integration-operator/src/controllers WorkloadRegistry
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/spireclient/svids/mock.go github.com/otterize/spire-integration-operator/src/controllers/spireclient/svids Store
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -package mock_secrets -destination=mocks/controllers/secrets/mock.go github.com/otterize/spire-integration-operator/src/controllers SecretsManager
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/eventrecorder/mock.go k8s.io/client-go/tools/record EventRecorder
