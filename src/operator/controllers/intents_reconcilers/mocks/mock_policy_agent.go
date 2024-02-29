// Code generated by MockGen. DO NOT EDIT.
// Source: ./iam_reconciler.go

// Package intentsreconcilersmocks is a generated GoMock package.
package intentsreconcilersmocks

import (
	context "context"
	reflect "reflect"

	v1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	gomock "go.uber.org/mock/gomock"
)

// MockIAMPolicyAgent is a mock of IAMPolicyAgent interface.
type MockIAMPolicyAgent struct {
	ctrl     *gomock.Controller
	recorder *MockIAMPolicyAgentMockRecorder
}

// MockIAMPolicyAgentMockRecorder is the mock recorder for MockIAMPolicyAgent.
type MockIAMPolicyAgentMockRecorder struct {
	mock *MockIAMPolicyAgent
}

// NewMockIAMPolicyAgent creates a new mock instance.
func NewMockIAMPolicyAgent(ctrl *gomock.Controller) *MockIAMPolicyAgent {
	mock := &MockIAMPolicyAgent{ctrl: ctrl}
	mock.recorder = &MockIAMPolicyAgentMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockIAMPolicyAgent) EXPECT() *MockIAMPolicyAgentMockRecorder {
	return m.recorder
}

// AddRolePolicyFromIntents mocks base method.
func (m *MockIAMPolicyAgent) AddRolePolicyFromIntents(ctx context.Context, namespace, accountName, intentsServiceName string, intents []v1alpha3.Intent) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AddRolePolicyFromIntents", ctx, namespace, accountName, intentsServiceName, intents)
	ret0, _ := ret[0].(error)
	return ret0
}

// AddRolePolicyFromIntents indicates an expected call of AddRolePolicyFromIntents.
func (mr *MockIAMPolicyAgentMockRecorder) AddRolePolicyFromIntents(ctx, namespace, accountName, intentsServiceName, intents interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AddRolePolicyFromIntents", reflect.TypeOf((*MockIAMPolicyAgent)(nil).AddRolePolicyFromIntents), ctx, namespace, accountName, intentsServiceName, intents)
}

// ApplyOnPodLabel mocks base method.
func (m *MockIAMPolicyAgent) ApplyOnPodLabel() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ApplyOnPodLabel")
	ret0, _ := ret[0].(string)
	return ret0
}

// ApplyOnPodLabel indicates an expected call of ApplyOnPodLabel.
func (mr *MockIAMPolicyAgentMockRecorder) ApplyOnPodLabel() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ApplyOnPodLabel", reflect.TypeOf((*MockIAMPolicyAgent)(nil).ApplyOnPodLabel))
}

// DeleteRolePolicyFromIntents mocks base method.
func (m *MockIAMPolicyAgent) DeleteRolePolicyFromIntents(ctx context.Context, intents v1alpha3.ClientIntents) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteRolePolicyFromIntents", ctx, intents)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteRolePolicyFromIntents indicates an expected call of DeleteRolePolicyFromIntents.
func (mr *MockIAMPolicyAgentMockRecorder) DeleteRolePolicyFromIntents(ctx, intents interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteRolePolicyFromIntents", reflect.TypeOf((*MockIAMPolicyAgent)(nil).DeleteRolePolicyFromIntents), ctx, intents)
}

// IntentType mocks base method.
func (m *MockIAMPolicyAgent) IntentType() v1alpha3.IntentType {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "IntentType")
	ret0, _ := ret[0].(v1alpha3.IntentType)
	return ret0
}

// IntentType indicates an expected call of IntentType.
func (mr *MockIAMPolicyAgentMockRecorder) IntentType() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "IntentType", reflect.TypeOf((*MockIAMPolicyAgent)(nil).IntentType))
}