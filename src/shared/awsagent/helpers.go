package awsagent

import (
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/samber/lo"
)

func HasSoftDeleteStrategyTagSet(tags []types.Tag) bool {
	return lo.SomeBy(tags, func(tag types.Tag) bool {
		return lo.FromPtr(tag.Key) == softDeletionStrategyTagKey && lo.FromPtr(tag.Value) == softDeletionStrategyTagValue
	})
}

func hasSoftDeletedTagSet(tags []types.Tag) bool {
	return lo.SomeBy(tags, func(tag types.Tag) bool {
		return lo.FromPtr(tag.Key) == softDeletedTagKey && lo.FromPtr(tag.Value) == softDeletedTagValue
	})
}
