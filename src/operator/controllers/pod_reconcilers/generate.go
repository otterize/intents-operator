package pod_reconcilers

//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination mocks/service_effective_policy_reconciler.go -package=podreconcilersmocks -source=pods.go GroupReconciler
