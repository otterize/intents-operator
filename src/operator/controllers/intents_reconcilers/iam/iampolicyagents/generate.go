package iampolicyagents

//go:generate go run go.uber.org/mock/mockgen@v0.2.0 -destination=./mocks/mock_policy_agent.go -package=mockiampolicyagents -source=./interface.go IAMPolicyAgent
