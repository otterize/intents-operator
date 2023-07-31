package protected_services_reconcilers

//go:generate go run go.uber.org/mock/mockgen -package protectedservicesmock -destination ./mocks/ext_netpol_handler_mock.go -source ./default_deny.go ExternalNepolHandler
