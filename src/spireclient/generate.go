package spireclient

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/client/mock.go -source=client.go
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/bundlev1/mock.go github.com/spiffe/spire-api-sdk/proto/spire/api/server/bundle/v1 BundleClient
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/svidv1/mock.go github.com/spiffe/spire-api-sdk/proto/spire/api/server/svid/v1 SVIDClient
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -destination=mocks/entryv1/mock.go github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1 EntryClient
