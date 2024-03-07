package main

//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=mocks/bundlev1/mock.go github.com/spiffe/spire-api-sdk/proto/spire/api/server/bundle/v1 BundleClient
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=mocks/svidv1/mock.go github.com/spiffe/spire-api-sdk/proto/spire/api/server/svid/v1 SVIDClient
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=mocks/entryv1/mock.go github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1 EntryClient
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=mocks/controller-runtime/client/mock.go sigs.k8s.io/controller-runtime/pkg/client Client,Reader,Writer
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=mocks/spireclient/mock.go github.com/otterize/credentials-operator/src/controllers/spireclient ServerClient
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=mocks/spireclient/bundles/mock.go github.com/otterize/credentials-operator/src/controllers/spireclient/bundles Store
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -package mock_entries -destination=mocks/entries/mock.go github.com/otterize/credentials-operator/src/controllers WorkloadRegistry
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=mocks/spireclient/svids/mock.go github.com/otterize/credentials-operator/src/controllers/spireclient/svids Store
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -package mock_secrets -destination=mocks/controllers/secrets/mock.go github.com/otterize/credentials-operator/src/controllers SecretsManager
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -package mockserviceaccounts -destination=mocks/controllers/serviceaccounts/mock.go github.com/otterize/credentials-operator/src/controllers ServiceAccountEnsurer
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=mocks/eventrecorder/mock.go k8s.io/client-go/tools/record EventRecorder
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=mocks/serviceidresolver/mock.go github.com/otterize/credentials-operator/src/controllers/secrets/types ServiceIdResolver
//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=mocks/gcp/mock2.go -package=gcp_service_accounts -source=./controllers/gcp_iam/gcp_service_accounts/gcp_service_accounts_controller.go GCPServiceAccountManager
