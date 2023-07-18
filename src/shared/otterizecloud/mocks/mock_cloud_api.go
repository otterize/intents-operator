// Code generated by MockGen. DO NOT EDIT.
// Source: ../../operator/controllers/intents_reconcilers/otterizecloud/cloud_api.go

// Package otterizecloudmocks is a generated GoMock package.
package otterizecloudmocks

import (
	context "context"
	reflect "reflect"

	graphqlclient "github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	gomock "go.uber.org/mock/gomock"
)

// MockCloudClient is a mock of CloudClient interface.
type MockCloudClient struct {
	ctrl     *gomock.Controller
	recorder *MockCloudClientMockRecorder
}

// MockCloudClientMockRecorder is the mock recorder for MockCloudClient.
type MockCloudClientMockRecorder struct {
	mock *MockCloudClient
}

// NewMockCloudClient creates a new mock instance.
func NewMockCloudClient(ctrl *gomock.Controller) *MockCloudClient {
	mock := &MockCloudClient{ctrl: ctrl}
	mock.recorder = &MockCloudClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockCloudClient) EXPECT() *MockCloudClientMockRecorder {
	return m.recorder
}

// ReportAppliedIntents mocks base method.
func (m *MockCloudClient) ReportAppliedIntents(ctx context.Context, namespace *string, intents []*graphqlclient.IntentInput) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReportAppliedIntents", ctx, namespace, intents)
	ret0, _ := ret[0].(error)
	return ret0
}

// ReportAppliedIntents indicates an expected call of ReportAppliedIntents.
func (mr *MockCloudClientMockRecorder) ReportAppliedIntents(ctx, namespace, intents interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReportAppliedIntents", reflect.TypeOf((*MockCloudClient)(nil).ReportAppliedIntents), ctx, namespace, intents)
}

// ReportComponentStatus mocks base method.
func (m *MockCloudClient) ReportComponentStatus(ctx context.Context, component graphqlclient.ComponentType) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ReportComponentStatus", ctx, component)
}

// ReportComponentStatus indicates an expected call of ReportComponentStatus.
func (mr *MockCloudClientMockRecorder) ReportComponentStatus(ctx, component interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReportComponentStatus", reflect.TypeOf((*MockCloudClient)(nil).ReportComponentStatus), ctx, component)
}

// ReportIntentsOperatorConfiguration mocks base method.
func (m *MockCloudClient) ReportIntentsOperatorConfiguration(ctx context.Context, config graphqlclient.IntentsOperatorConfigurationInput) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReportIntentsOperatorConfiguration", ctx, config)
	ret0, _ := ret[0].(error)
	return ret0
}

// ReportIntentsOperatorConfiguration indicates an expected call of ReportIntentsOperatorConfiguration.
func (mr *MockCloudClientMockRecorder) ReportIntentsOperatorConfiguration(ctx, config interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReportIntentsOperatorConfiguration", reflect.TypeOf((*MockCloudClient)(nil).ReportIntentsOperatorConfiguration), ctx, config)
}

// ReportKafkaServerConfig mocks base method.
func (m *MockCloudClient) ReportKafkaServerConfig(ctx context.Context, namespace string, servers []graphqlclient.KafkaServerConfigInput) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReportKafkaServerConfig", ctx, namespace, servers)
	ret0, _ := ret[0].(error)
	return ret0
}

// ReportKafkaServerConfig indicates an expected call of ReportKafkaServerConfig.
func (mr *MockCloudClientMockRecorder) ReportKafkaServerConfig(ctx, namespace, servers interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReportKafkaServerConfig", reflect.TypeOf((*MockCloudClient)(nil).ReportKafkaServerConfig), ctx, namespace, servers)
}
