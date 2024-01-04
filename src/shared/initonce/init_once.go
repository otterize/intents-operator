package initonce

import (
	"github.com/otterize/intents-operator/src/shared/errors"
	"sync"
)

type InitOnce struct {
	once sync.Once
}

// Do call the function f if Do is being called for the first time for this instance of InitOnce or if previous calls
// to Do have failed. After the first successful call to f, subsequent calls to Do won't get executed and the error
// returned by Do will nil.
func (i *InitOnce) Do(f func() error) error {
	var err error
	i.once.Do(func() {
		err = f()
	})
	if err != nil {
		i.once = sync.Once{}
		return errors.Wrap(err)
	}
	return nil
}
