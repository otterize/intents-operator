package azureagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
)

// NewListPager creates a new Azure core Pager for a list of items.
func NewListPager[T any](items ...T) *runtime.Pager[T] {
	current := 0
	return runtime.NewPager[T](runtime.PagingHandler[T]{
		More: func(page T) bool {
			return len(items) > current
		},
		Fetcher: func(ctx context.Context, page *T) (T, error) {
			if len(items) <= current {
				return *new(T), nil
			}
			item := items[current]
			current++
			return item, nil
		},
	})
}
