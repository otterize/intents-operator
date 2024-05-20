package intentsreconcilersmocks

import (
	"context"
	ctrl "sigs.k8s.io/controller-runtime"
)

type MockIntentsReconcilerForTestEnv struct{}

func (m *MockIntentsReconcilerForTestEnv) Reconcile(context.Context, ctrl.Request) (ctrl.Result, error) {
	// Mock struct just to satisfy constructor needs for now
	return ctrl.Result{}, nil
}
