package basicbatch

// it's britney batch

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
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
	flushChan     chan chan struct{}
}

func NewBatcher[T any](handler func(batch []T) error, minDelay time.Duration, maxBatchSize int, bufferSize int) *Batcher[T] {
	return &Batcher[T]{
		handleBatch:  handler,
		minDelay:     minDelay,
		items:        make(chan T, bufferSize),
		maxBatchSize: maxBatchSize,
		flushChan:    make(chan chan struct{}, 100),
	}
}

func (b *Batcher[T]) Flush(ctx context.Context) error {
	flushSignal := make(chan struct{})
	b.flushChan <- flushSignal
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-flushSignal:
		return nil
	}
}

func (b *Batcher[T]) getBatch() (items []T, flushSignal chan struct{}) {
	for {
		select {
		case item := <-b.items:
			items = append(items, item)
			select {
			case flushSignal = <-b.flushChan:
			default:
			}
			if len(items) == b.maxBatchSize || time.Now().After(b.lastExecution.Add(b.minDelay)) {
				time.Sleep(time.Until(b.lastExecution.Add(b.minDelay)))
				return items, flushSignal
			}
		case <-time.After(b.minDelay):
			if len(items) == 0 {
				continue
			}
			return items, flushSignal
		}
	}
}

func (b *Batcher[T]) runForever() {
	defer func() {
		r := recover()
		if r != nil {
			logger := logrus.WithField("panic", r)
			if rErr, ok := r.(error); ok {
				logger = logger.WithError(rErr)
			}
			logger.Error("recovered from panic in batcher: %r")
		}
		panic(r)
	}()
	for {
		batchItems, flushSignal := b.getBatch()
		err := b.handleBatchWithPanicRecovery(batchItems)
		if err != nil {
			logrus.WithError(err).Error("failed handling batch, skipping")
		}
		if flushSignal != nil {
			close(flushSignal)
		}
		b.lastExecution = time.Now()
	}
}

func (b *Batcher[T]) handleBatchWithPanicRecovery(batch []T) (err error) {
	defer func() {
		r := recover()
		if r != nil {
			logger := logrus.WithField("panic", r)
			if rErr, ok := r.(error); ok {
				err = rErr
			}
			if err == nil {
				err = errors.Errorf("recovered from panic in batcher: %r", r)
			}
			logger.WithError(err).Error("recovered from panic in batcher")
			return
		}
	}()
	err = b.handleBatch(batch)
	if err != nil {
		logrus.WithError(err).Error("failed handling batch")
	}
	return err
}

func (b *Batcher[T]) AddNoWait(item T) bool {
	b.startOnce.Do(func() {
		go b.runForever()
	})
	select {
	case b.items <- item:
		return true
	default:
		return false
	}
}
