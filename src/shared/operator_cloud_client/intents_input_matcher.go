package operator_cloud_client

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"golang.org/x/exp/constraints"
	"sort"
)

// IntentsMatcher Implement gomock.Matcher interface for []graphqlclient.IntentInput
type IntentsMatcher struct {
	expected []graphqlclient.IntentInput
}

func NilCompare[T constraints.Ordered](a *T, b *T) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	if *a > *b {
		return 1
	}
	if *a < *b {
		return -1
	}
	return 0
}

func intentInputSort(intents []graphqlclient.IntentInput) {
	for _, intent := range intents {
		if intent.Type == nil {
			continue
		}

		switch *intent.Type {
		case graphqlclient.IntentTypeKafka:
			for _, topic := range intent.Topics {
				sort.Slice(topic.Operations, func(i, j int) bool {
					return NilCompare(topic.Operations[i], topic.Operations[j]) < 0
				})
			}
			sort.Slice(intent.Topics, func(i, j int) bool {
				res := NilCompare(intent.Topics[i].Name, intent.Topics[j].Name)
				if res != 0 {
					return res < 0
				}

				return len(intent.Topics[i].Operations) < len(intent.Topics[j].Operations)
			})
		case graphqlclient.IntentTypeHttp:
			for _, resource := range intent.Resources {
				sort.Slice(resource.Methods, func(i, j int) bool {
					return NilCompare(resource.Methods[i], resource.Methods[j]) < 0
				})
			}
			sort.Slice(intent.Resources, func(i, j int) bool {
				res := NilCompare(intent.Resources[i].Path, intent.Resources[j].Path)
				if res != 0 {
					return res < 0
				}

				return len(intent.Resources[i].Methods) < len(intent.Resources[j].Methods)
			})
		case graphqlclient.IntentTypeInternet:
			sort.Slice(intent.Internet.Ips, func(i, j int) bool {
				return NilCompare(intent.Internet.Ips[i], intent.Internet.Ips[j]) < 0
			})
			if intent.Internet.Ports != nil {
				sort.Slice(intent.Internet.Ports, func(i, j int) bool {
					return NilCompare(intent.Internet.Ports[i], intent.Internet.Ports[j]) < 0
				})
			}
		}
	}
	sort.Slice(intents, func(i, j int) bool {
		res := NilCompare(intents[i].Namespace, intents[j].Namespace)
		if res != 0 {
			return res < 0
		}
		res = NilCompare(intents[i].ClientName, intents[j].ClientName)
		if res != 0 {
			return res < 0
		}
		res = NilCompare(intents[i].ServerName, intents[j].ServerName)
		if res != 0 {
			return res < 0
		}
		res = NilCompare(intents[i].ServerNamespace, intents[j].ServerNamespace)
		if res != 0 {
			return res < 0
		}
		res = NilCompare(intents[i].Type, intents[j].Type)
		if res != 0 {
			return res < 0
		}
		switch *intents[i].Type {
		case graphqlclient.IntentTypeKafka:
			return len(intents[i].Topics) < len(intents[j].Topics)
		case graphqlclient.IntentTypeHttp:
			return len(intents[i].Resources) < len(intents[j].Resources)
		case graphqlclient.IntentTypeInternet:
			if len(intents[i].Internet.Ips) == len(intents[j].Internet.Ips) {
				return len(intents[i].Internet.Ports) < len(intents[j].Internet.Ports)
			}
			return len(intents[i].Internet.Ips) < len(intents[j].Internet.Ips)
		default:
			panic("Unimplemented intent type")
		}
	})
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

	intentInputSort(expectedIntents)
	intentInputSort(actualIntents)

	diff := cmp.Diff(expectedIntents, actualIntents)
	if diff != "" {
		fmt.Println(diff)
	}
	return cmp.Equal(expectedIntents, actualIntents)
}

func (m IntentsMatcher) String() string {
	return prettyPrint(m)
}

func printKafkaConfigInput(input []*graphqlclient.KafkaConfigInput) string {
	var result string
	for _, item := range input {
		var name, operations string
		if item == nil {
			result += "nil"
			continue
		}
		if item.Name != nil {
			name = *item.Name
		} else {
			name = "nil"
		}

		for _, op := range item.Operations {
			if op == nil {
				operations += "nil,"
				continue
			}
			operations += fmt.Sprintf("%s,", *op)
		}
		result += fmt.Sprintf("KafkaConfigInput{Name: %s, Operations: %v}", name, operations)
	}
	return result
}

func prettyPrint(m IntentsMatcher) string {
	expected := m.expected
	var result string
	itemFormat := "IntentInput{ClientName: %s, ServerName: %s, Namespace: %s, ServerNamespace: %s, Type: %s, Resource: %s, Status: %s},"
	for _, intent := range expected {
		var clientName, namespace, serverName, serverNamespace, intentType, resource, status string
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
			switch *intent.Type {
			case graphqlclient.IntentTypeKafka:
				resource = printKafkaConfigInput(intent.Topics)
			case graphqlclient.IntentTypeHttp:
				// Fallthrough
			default:
				resource = "Unimplemented type"
			}
		}
		if intent.Status != nil && intent.Status.IstioStatus != nil {
			status = fmt.Sprintf("sa: %s, isShared: %t", *intent.Status.IstioStatus.ServiceAccountName, *intent.Status.IstioStatus.IsServiceAccountShared)
		}
		result += fmt.Sprintf(itemFormat, clientName, serverName, namespace, serverNamespace, intentType, resource, status)
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
