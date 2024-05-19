// Code generated by MockGen. DO NOT EDIT.
// Source: ../../../shared/serviceidresolver/serviceidresolver.go

// Package intentsreconcilersmocks is a generated GoMock package.
package intentsreconcilersmocks

import (
	context "context"
	reflect "reflect"

	v1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	serviceidentity "github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	gomock "go.uber.org/mock/gomock"
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

// ResolveClientIntentToPod mocks base method.
func (m *MockServiceResolver) ResolveClientIntentToPod(ctx context.Context, intent v1alpha3.ClientIntents) (v1.Pod, error) {
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

// ResolvePodToServiceIdentity mocks base method.
func (m *MockServiceResolver) ResolvePodToServiceIdentity(ctx context.Context, pod *v1.Pod) (serviceidentity.ServiceIdentity, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ResolvePodToServiceIdentity", ctx, pod)
	ret0, _ := ret[0].(serviceidentity.ServiceIdentity)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ResolvePodToServiceIdentity indicates an expected call of ResolvePodToServiceIdentity.
func (mr *MockServiceResolverMockRecorder) ResolvePodToServiceIdentity(ctx, pod interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ResolvePodToServiceIdentity", reflect.TypeOf((*MockServiceResolver)(nil).ResolvePodToServiceIdentity), ctx, pod)
}

// ResolveServiceIdentityToPodSlice mocks base method.
func (m *MockServiceResolver) ResolveServiceIdentityToPodSlice(ctx context.Context, identity *serviceidentity.ServiceIdentity) ([]v1.Pod, bool, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ResolveServiceIdentityToPodSlice", ctx, identity)
	ret0, _ := ret[0].([]v1.Pod)
	ret1, _ := ret[1].(bool)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// ResolveServiceIdentityToPodSlice indicates an expected call of ResolveServiceIdentityToPodSlice.
func (mr *MockServiceResolverMockRecorder) ResolveServiceIdentityToPodSlice(ctx, identity interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ResolveServiceIdentityToPodSlice", reflect.TypeOf((*MockServiceResolver)(nil).ResolveServiceIdentityToPodSlice), ctx, identity)
}
