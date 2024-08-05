package intents_reconcilers

import (
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClientPatch struct {
	client.Patch
}

func (p ClientPatch) Matches(x interface{}) bool {
	patch := x.(client.Patch)
	actualData, err := patch.Data(&DummyObject{})
	if err != nil {
		return false
	}

	expectedData, err := p.Data(&DummyObject{})
	if err != nil {
		return false
	}

	return string(actualData) == string(expectedData) && patch.Type() == p.Type()
}

func (p ClientPatch) String() string {
	data, err := p.Data(&DummyObject{})
	if err != nil {
		return "format error"
	}
	return string(data)
}

func MatchPatch(patch client.Patch) gomock.Matcher {
	return ClientPatch{patch}
}

// DummyObject is a placeholder to avoid nil dereference
type DummyObject struct {
	v1.Pod
}
