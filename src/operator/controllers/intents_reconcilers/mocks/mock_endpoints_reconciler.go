// Code generated by MockGen. DO NOT EDIT.
// Source: ../external_traffic/endpoints_reconciler.go

// Package intentsreconcilersmocks is a generated GoMock package.
package intentsreconcilersmocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

// MockEndpointsReconcilerInterface is a mock of EndpointsReconcilerInterface interface.
type MockEndpointsReconcilerInterface struct {
	ctrl     *gomock.Controller
	recorder *MockEndpointsReconcilerInterfaceMockRecorder
}

// MockEndpointsReconcilerInterfaceMockRecorder is the mock recorder for MockEndpointsReconcilerInterface.
type MockEndpointsReconcilerInterfaceMockRecorder struct {
	mock *MockEndpointsReconcilerInterface
}

// NewMockEndpointsReconcilerInterface creates a new mock instance.
func NewMockEndpointsReconcilerInterface(ctrl *gomock.Controller) *MockEndpointsReconcilerInterface {
	mock := &MockEndpointsReconcilerInterface{ctrl: ctrl}
	mock.recorder = &MockEndpointsReconcilerInterfaceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockEndpointsReconcilerInterface) EXPECT() *MockEndpointsReconcilerInterfaceMockRecorder {
	return m.recorder
}

// Reconcile mocks base method.
func (m *MockEndpointsReconcilerInterface) Reconcile(ctx context.Context, req controllerruntime.Request) (controllerruntime.Result, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Reconcile", ctx, req)
	ret0, _ := ret[0].(controllerruntime.Result)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Reconcile indicates an expected call of Reconcile.
func (mr *MockEndpointsReconcilerInterfaceMockRecorder) Reconcile(ctx, req interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Reconcile", reflect.TypeOf((*MockEndpointsReconcilerInterface)(nil).Reconcile), ctx, req)
}