// Code generated by MockGen. DO NOT EDIT.
// Source: controllers/linkerd/linkerd_manager.go

// Package intentsreconcilersmocks is a generated GoMock package.
package intentsreconcilersmocks

import (
	context "context"
	reflect "reflect"

	v1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	gomock "go.uber.org/mock/gomock"
)

// MockLinkerdPolicyManager is a mock of LinkerdPolicyManager interface.
type MockLinkerdPolicyManager struct {
	ctrl     *gomock.Controller
	recorder *MockLinkerdPolicyManagerMockRecorder
}

// MockLinkerdPolicyManagerMockRecorder is the mock recorder for MockLinkerdPolicyManager.
type MockLinkerdPolicyManagerMockRecorder struct {
	mock *MockLinkerdPolicyManager
}

// NewMockLinkerdPolicyManager creates a new mock instance.
func NewMockLinkerdPolicyManager(ctrl *gomock.Controller) *MockLinkerdPolicyManager {
	mock := &MockLinkerdPolicyManager{ctrl: ctrl}
	mock.recorder = &MockLinkerdPolicyManagerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockLinkerdPolicyManager) EXPECT() *MockLinkerdPolicyManagerMockRecorder {
	return m.recorder
}

// Create mocks base method.
func (m *MockLinkerdPolicyManager) Create(ctx context.Context, clientIntents *v1alpha3.ClientIntents, clientServiceAccount string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Create", ctx, clientIntents, clientServiceAccount)
	ret0, _ := ret[0].(error)
	return ret0
}

// Create indicates an expected call of Create.
func (mr *MockLinkerdPolicyManagerMockRecorder) Create(ctx, clientIntents, clientServiceAccount interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Create", reflect.TypeOf((*MockLinkerdPolicyManager)(nil).Create), ctx, clientIntents, clientServiceAccount)
}

// DeleteAll mocks base method.
func (m *MockLinkerdPolicyManager) DeleteAll(ctx context.Context, clientIntents *v1alpha3.ClientIntents) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteAll", ctx, clientIntents)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteAll indicates an expected call of DeleteAll.
func (mr *MockLinkerdPolicyManagerMockRecorder) DeleteAll(ctx, clientIntents interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteAll", reflect.TypeOf((*MockLinkerdPolicyManager)(nil).DeleteAll), ctx, clientIntents)
}
