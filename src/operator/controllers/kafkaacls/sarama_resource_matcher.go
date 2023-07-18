package kafkaacls

import (
	"fmt"
	"github.com/Shopify/sarama"
	"go.uber.org/mock/gomock"
	"golang.org/x/exp/slices"
	"reflect"
)

// Implement gomock matcher interface for sarama.ResourceAcls

type ResourceAclsMatcher struct {
	resourceAcls []*sarama.ResourceAcls
}

func (m *ResourceAclsMatcher) Matches(x interface{}) bool {
	actual, ok := x.([]*sarama.ResourceAcls)
	if !ok {
		return false
	}

	if len(actual) != len(m.resourceAcls) {
		return false
	}

	for i := range actual {
		if !compareResourceAcls(actual[i], m.resourceAcls[i]) {
			return false
		}
	}

	return true
}

func compareResourceAcls(a, b *sarama.ResourceAcls) bool {
	if a.Resource.ResourceType != b.Resource.ResourceType {
		return false
	}

	if a.Resource.ResourceName != b.Resource.ResourceName {
		return false
	}

	if len(a.Acls) != len(b.Acls) {
		return false
	}

	slices.SortFunc(a.Acls, sortAcl)
	slices.SortFunc(b.Acls, sortAcl)
	for i := range a.Acls {
		if !reflect.DeepEqual(a.Acls[i], b.Acls[i]) {
			return false
		}
	}

	return true
}

func sortAcl(i, j *sarama.Acl) bool {
	if i.Operation != j.Operation {
		return i.Operation < j.Operation
	}
	if i.PermissionType != j.PermissionType {
		return i.PermissionType < j.PermissionType
	}
	if i.Principal != j.Principal {
		return i.Principal < j.Principal
	}
	return i.Host < j.Host
}

func (m *ResourceAclsMatcher) String() string {
	var res string
	for _, resourceAcl := range m.resourceAcls {
		res += fmt.Sprintf("%+v\n", resourceAcl.Resource)
		for _, acl := range resourceAcl.Acls {
			res += fmt.Sprintf("%+v\n", acl)
		}
	}
	return res
}

func MatchResourceAcls(resourceAcls []*sarama.ResourceAcls) gomock.Matcher {
	return &ResourceAclsMatcher{resourceAcls}
}
