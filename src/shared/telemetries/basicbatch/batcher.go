package basicbatch

// it's britney batch

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

type Batcher[T any] struct {
	handleBatch   func([]T) error
	minDelay      time.Duration
	lastExecution time.Time
	items         chan T
	startOnce     sync.Once
	maxBatchSize  int
}

func NewBatcher[T any](handler func(batch []T) error, minDelay time.Duration, maxBatchSize int, bufferSize int) *Batcher[T] {
	return &Batcher[T]{
		handleBatch:  handler,
		minDelay:     minDelay,
		items:        make(chan T, bufferSize),
		maxBatchSize: maxBatchSize,
	}
}

func (b *Batcher[T]) getBatch() []T {
	items := make([]T, 0)

	for {
		select {
		case item := <-b.items:
			items = append(items, item)
			if len(items) == b.maxBatchSize || time.Now().After(b.lastExecution.Add(b.minDelay)) {
				time.Sleep(time.Until(b.lastExecution.Add(b.minDelay)))
				return items
			}
		case <-time.After(b.minDelay):
			if len(items) == 0 {
				continue
			}
			return items
		}
	}
}

func (b *Batcher[T]) runForever() {
	defer func() {
		r := recover()
		if r != nil {
			logrus.Error("recovered from panic in batcher")
		}
	}()
	for {
		batchItems := b.getBatch()
		err := b.handleBatch(batchItems)
		if err != nil {
			logrus.WithError(err).Error("failed handling batch, skipping")
		}
		b.lastExecution = time.Now()
	}
}
func (b *Batcher[T]) AddNoWait(item T) bool {
	b.startOnce.Do(func() {
		errorreporter.RunWithErrorReportAndRecover(context.Background(), "batcher", func(_ context.Context) {
			b.runForever()
		})
	})
	select {
	case b.items <- item:
		return true
	default:
		return false
	}
}
