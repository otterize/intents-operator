// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/networkpolicy (interfaces: ExternalNetpolHandler)

// Package intentsreconcilersmocks is a generated GoMock package.
package intentsreconcilersmocks

import (
	context "context"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
	v1 "k8s.io/api/networking/v1"
	labels "k8s.io/apimachinery/pkg/labels"
)

// MockExternalNetpolHandler is a mock of ExternalNetpolHandler interface.
type MockExternalNetpolHandler struct {
	ctrl     *gomock.Controller
	recorder *MockExternalNetpolHandlerMockRecorder
}

// MockExternalNetpolHandlerMockRecorder is the mock recorder for MockExternalNetpolHandler.
type MockExternalNetpolHandlerMockRecorder struct {
	mock *MockExternalNetpolHandler
}

// NewMockExternalNetpolHandler creates a new mock instance.
func NewMockExternalNetpolHandler(ctrl *gomock.Controller) *MockExternalNetpolHandler {
	mock := &MockExternalNetpolHandler{ctrl: ctrl}
	mock.recorder = &MockExternalNetpolHandlerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockExternalNetpolHandler) EXPECT() *MockExternalNetpolHandlerMockRecorder {
	return m.recorder
}

// HandleAllPods mocks base method.
func (m *MockExternalNetpolHandler) HandleAllPods(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HandleAllPods", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// HandleAllPods indicates an expected call of HandleAllPods.
func (mr *MockExternalNetpolHandlerMockRecorder) HandleAllPods(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HandleAllPods", reflect.TypeOf((*MockExternalNetpolHandler)(nil).HandleAllPods), arg0)
}

// HandleBeforeAccessPolicyRemoval mocks base method.
func (m *MockExternalNetpolHandler) HandleBeforeAccessPolicyRemoval(arg0 context.Context, arg1 *v1.NetworkPolicy) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HandleBeforeAccessPolicyRemoval", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// HandleBeforeAccessPolicyRemoval indicates an expected call of HandleBeforeAccessPolicyRemoval.
func (mr *MockExternalNetpolHandlerMockRecorder) HandleBeforeAccessPolicyRemoval(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HandleBeforeAccessPolicyRemoval", reflect.TypeOf((*MockExternalNetpolHandler)(nil).HandleBeforeAccessPolicyRemoval), arg0, arg1)
}

// HandlePodsByLabelSelector mocks base method.
func (m *MockExternalNetpolHandler) HandlePodsByLabelSelector(arg0 context.Context, arg1 string, arg2 labels.Selector) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HandlePodsByLabelSelector", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// HandlePodsByLabelSelector indicates an expected call of HandlePodsByLabelSelector.
func (mr *MockExternalNetpolHandlerMockRecorder) HandlePodsByLabelSelector(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HandlePodsByLabelSelector", reflect.TypeOf((*MockExternalNetpolHandler)(nil).HandlePodsByLabelSelector), arg0, arg1, arg2)
}
