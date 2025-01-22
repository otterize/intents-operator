package awscredentialsagent

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/awsagent/multi_account_aws_agent"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	corev1 "k8s.io/api/core/v1"
)

type MultiaccountAWSCredentialsAgent struct {
	agents map[string]*Agent
}

func NewMultiaccountAWSCredentialsAgent(ctx context.Context, accounts []operatorconfig.AWSAccount, awsOptions []awsagent.Option, clusterName string, keyPath string, certPath string) (*MultiaccountAWSCredentialsAgent, error) {
	agents, err := multi_account_aws_agent.MakeAgentsFromAccountList(ctx, accounts, NewAWSCredentialsAgent, awsOptions, clusterName, keyPath, certPath)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	return &MultiaccountAWSCredentialsAgent{agents: agents}, nil
}

func (m *MultiaccountAWSCredentialsAgent) AppliesOnPod(pod *corev1.Pod) bool {
	return multi_account_aws_agent.AppliesOnPod(pod)
}

func (m *MultiaccountAWSCredentialsAgent) FinalizerName() string {
	return AWSAgentFinalizer
}

func (m *MultiaccountAWSCredentialsAgent) ServiceAccountLabel() string {
	return AWSOtterizeServiceAccountLabel
}

func (m *MultiaccountAWSCredentialsAgent) OnPodAdmission(ctx context.Context, pod *corev1.Pod, serviceAccount *corev1.ServiceAccount, dryRun bool) error {
	agent, err := m.getAgentForPod(pod)
	if err != nil {
		return errors.Wrap(err)
	}

	return agent.OnPodAdmission(ctx, pod, serviceAccount, dryRun)
}

func (m *MultiaccountAWSCredentialsAgent) getAgentForPod(pod *corev1.Pod) (*Agent, error) {
	accountId, found := multi_account_aws_agent.AccountFromPod(pod)
	if !found {
		return nil, errors.New("AWS account must be specified on pod")
	}
	agent, ok := m.agents[accountId]
	if !ok {
		return nil, errors.Errorf("AWS account ID '%s' is not configured", accountId)
	}
	return agent, nil
}

func (m *MultiaccountAWSCredentialsAgent) getAgentForServiceAccount(serviceAccount *corev1.ServiceAccount) (*Agent, error) {
	accountId, found := multi_account_aws_agent.AccountFromServiceAccount(serviceAccount)
	if !found {
		return nil, errors.New("AWS account must be specified on service account")
	}
	agent, ok := m.agents[accountId]
	if !ok {
		return nil, errors.Errorf("AWS account ID '%s' is not configured", accountId)
	}
	return agent, nil
}

func (m *MultiaccountAWSCredentialsAgent) OnPodUpdate(ctx context.Context, pod *corev1.Pod, serviceAccount *corev1.ServiceAccount) (updated bool, requeue bool, err error) {
	agent, err := m.getAgentForPod(pod)
	if err != nil {
		return false, false, errors.Wrap(err)
	}
	return agent.OnPodUpdate(ctx, pod, serviceAccount)
}

func (m *MultiaccountAWSCredentialsAgent) OnServiceAccountUpdate(ctx context.Context, serviceAccount *corev1.ServiceAccount) (updated bool, requeue bool, err error) {
	agent, err := m.getAgentForServiceAccount(serviceAccount)
	if err != nil {
		return false, false, errors.Wrap(err)
	}
	return agent.OnServiceAccountUpdate(ctx, serviceAccount)
}

func (m *MultiaccountAWSCredentialsAgent) OnServiceAccountTermination(ctx context.Context, serviceAccount *corev1.ServiceAccount) error {
	agent, err := m.getAgentForServiceAccount(serviceAccount)
	if err != nil {
		return errors.Wrap(err)
	}
	return agent.OnServiceAccountTermination(ctx, serviceAccount)
}

// needs to implement: IAMPolicyAgent, IAMCredentialsAgent
