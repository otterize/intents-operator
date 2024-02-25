package awsagent

import (
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/samber/lo"
)

func HasMarkAsUnusedInCaseOfDeletionTagSet(tags []types.Tag) bool {
	return lo.SomeBy(tags, func(tag types.Tag) bool {
		return lo.FromPtr(tag.Key) == markAsUnusedInsteadOfDeleteTagKey && lo.FromPtr(tag.Value) == markAsUnusedInsteadOfDeleteValue
	})
}

func hasUnusedTagSet(tags []types.Tag) bool {
	return lo.SomeBy(tags, func(tag types.Tag) bool {
		return lo.FromPtr(tag.Key) == unusedTagKey && lo.FromPtr(tag.Value) == unusedTagValue
	})
}
