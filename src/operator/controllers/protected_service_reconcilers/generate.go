package protected_service_reconcilers

//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -package protectedservicesmock -destination ./mocks/ext_netpol_handler_mock.go -source ./default_deny.go ExternalNepolHandler
