package initonce

import (
	"errors"
	"github.com/stretchr/testify/suite"
	"testing"
)

type InitOnceSuite struct {
	suite.Suite
}

func (s *InitOnceSuite) TestDo() {
	var counter int
	var once InitOnce
	s.Equal(0, counter)
	s.NoError(once.Do(func() error {
		counter++
		return nil
	}))
	s.Equal(1, counter)
	s.NoError(once.Do(func() error {
		counter++
		return nil
	}))
	s.Equal(1, counter)
	s.NoError(once.Do(func() error {
		counter++
		return nil
	}))
	s.Equal(1, counter)
}

func (s *InitOnceSuite) TestDoWithError() {
	var counter int
	var once InitOnce
	s.Equal(0, counter)
	s.Error(once.Do(func() error {
		counter++
		return errors.New("test error")
	}))
	s.Equal(1, counter)
	s.Error(once.Do(func() error {
		counter++
		return errors.New("another test error")
	}))
	s.Equal(2, counter)
	s.NoError(once.Do(func() error {
		counter++
		return nil
	}))
	s.Equal(3, counter)
	s.NoError(once.Do(func() error {
		counter++
		return errors.New("this error will be ignored since the function will never be called")
	}))
	s.Equal(3, counter)
}

func TestInitOnceSuite(t *testing.T) {
	suite.Run(t, new(InitOnceSuite))
}
