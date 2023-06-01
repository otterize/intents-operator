// Code generated by MockGen. DO NOT EDIT.
// Source: ../../../shared/serviceidresolver/serviceidresolver.go

// Package intentsreconcilersmocks is a generated GoMock package.
package intentsreconcilersmocks

import (
	context "context"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	v1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	v1 "k8s.io/api/core/v1"
)

// MockServiceResolver is a mock of ServiceResolver interface.
type MockServiceResolver struct {
	ctrl     *gomock.Controller
	recorder *MockServiceResolverMockRecorder
}

// MockServiceResolverMockRecorder is the mock recorder for MockServiceResolver.
type MockServiceResolverMockRecorder struct {
	mock *MockServiceResolver
}

// NewMockServiceResolver creates a new mock instance.
func NewMockServiceResolver(ctrl *gomock.Controller) *MockServiceResolver {
	mock := &MockServiceResolver{ctrl: ctrl}
	mock.recorder = &MockServiceResolverMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockServiceResolver) EXPECT() *MockServiceResolverMockRecorder {
	return m.recorder
}

// GetPodAnnotatedName mocks base method.
func (m *MockServiceResolver) GetPodAnnotatedName(ctx context.Context, podName, podNamespace string) (string, bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetPodAnnotatedName", ctx, podName, podNamespace)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(bool)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetPodAnnotatedName indicates an expected call of GetPodAnnotatedName.
func (mr *MockServiceResolverMockRecorder) GetPodAnnotatedName(ctx, podName, podNamespace interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPodAnnotatedName", reflect.TypeOf((*MockServiceResolver)(nil).GetPodAnnotatedName), ctx, podName, podNamespace)
}

// ResolveClientIntentToPod mocks base method.
func (m *MockServiceResolver) ResolveClientIntentToPod(ctx context.Context, intent v1alpha2.ClientIntents) (v1.Pod, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ResolveClientIntentToPod", ctx, intent)
	ret0, _ := ret[0].(v1.Pod)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ResolveClientIntentToPod indicates an expected call of ResolveClientIntentToPod.
func (mr *MockServiceResolverMockRecorder) ResolveClientIntentToPod(ctx, intent interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ResolveClientIntentToPod", reflect.TypeOf((*MockServiceResolver)(nil).ResolveClientIntentToPod), ctx, intent)
}

// ResolveIntentServerToPod mocks base method.
func (m *MockServiceResolver) ResolveIntentServerToPod(ctx context.Context, intent v1alpha2.Intent, namespace string) (v1.Pod, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ResolveIntentServerToPod", ctx, intent, namespace)
	ret0, _ := ret[0].(v1.Pod)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ResolveIntentServerToPod indicates an expected call of ResolveIntentServerToPod.
func (mr *MockServiceResolverMockRecorder) ResolveIntentServerToPod(ctx, intent, namespace interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ResolveIntentServerToPod", reflect.TypeOf((*MockServiceResolver)(nil).ResolveIntentServerToPod), ctx, intent, namespace)
}
