package iam_pod_reconciler

//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/reconciler.go -package=mocks sigs.k8s.io/controller-runtime/pkg/reconcile Reconciler
