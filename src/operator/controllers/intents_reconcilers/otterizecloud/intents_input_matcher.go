package otterizecloud

import (
	"fmt"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
)

// IntentsMatcher Implement gomock.Matcher interface for []graphqlclient.IntentInput
type IntentsMatcher struct {
	expected []graphqlclient.IntentInput
}

func NilCompare[T comparable](a *T, b *T) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func compareIntentInput(a graphqlclient.IntentInput, b graphqlclient.IntentInput) bool {
	return NilCompare(a.ClientName, b.ClientName) &&
		NilCompare(a.Namespace, b.Namespace) &&
		NilCompare(a.ServerName, b.ServerName) &&
		NilCompare(a.ServerNamespace, b.ServerNamespace)
}

func (m IntentsMatcher) Matches(x interface{}) bool {
	if x == nil {
		return false
	}
	actualIntentsPtr, ok := x.([]*graphqlclient.IntentInput)
	if !ok {
		return false
	}
	matched := getMatcherFromPtrArr(actualIntentsPtr)
	actualIntents := matched.expected
	expectedIntents := m.expected
	if len(actualIntents) != len(expectedIntents) {
		return false
	}

	for _, expected := range expectedIntents {
		found := false
		for _, actual := range actualIntents {
			if compareIntentInput(actual, expected) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (m IntentsMatcher) String() string {
	return prettyPrint(m)
}

func prettyPrint(m IntentsMatcher) string {
	expected := m.expected
	var result string
	itemFormat := "IntentInput{ClientName: %s, ServerName: %s, Namespace: %s, ServerNamespace: %s, Type: %s},"
	for _, intent := range expected {
		var clientName, namespace, serverName, serverNamespace, intentType string
		if intent.ClientName != nil {
			clientName = *intent.ClientName
		}
		if intent.Namespace != nil {
			namespace = *intent.Namespace
		}
		if intent.ServerName != nil {
			serverName = *intent.ServerName
		}
		if intent.ServerNamespace != nil {
			serverNamespace = *intent.ServerNamespace
		}
		if intent.Type != nil {
			intentType = *intent.ServerNamespace
		}
		result += fmt.Sprintf(itemFormat, clientName, serverName, namespace, serverNamespace, intentType)
	}

	return result
}

func (m IntentsMatcher) Got(got interface{}) string {
	actual, ok := got.([]*graphqlclient.IntentInput)
	if !ok {
		return fmt.Sprintf("Not an []graphqlclient.IntentInput, Got: %v", got)
	}
	matched := getMatcherFromPtrArr(actual)
	return prettyPrint(matched)
}

func getMatcherFromPtrArr(actualPtr []*graphqlclient.IntentInput) IntentsMatcher {
	var actual []graphqlclient.IntentInput
	for _, intent := range actualPtr {
		actual = append(actual, *intent)
	}

	matched := IntentsMatcher{actual}
	return matched
}

func GetMatcher(expected []graphqlclient.IntentInput) IntentsMatcher {
	return IntentsMatcher{expected}
}
