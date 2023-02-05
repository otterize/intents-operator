package otterizecloudclient

import (
	"context"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"testing"
)

type CloudClientTestSuite struct {
	suite.Suite
}

func (s *CloudClientTestSuite) SetupTest() {
	viper.Reset()
}

func (s *CloudClientTestSuite) TearDownTest() {
	viper.Reset()
}

func (s *CloudClientTestSuite) TestNewClientSuccess() {
	viper.Set(ApiClientIdKey, "cli_id1234")
	viper.Set(ApiClientSecretKey, "secret1234")
	viper.Set(OtterizeAPIAddressKey, "https://testapp.otterize.com/api")
	viper.Set(CloudClientTimeoutKey, "30s")

	ctx := context.Background()
	client, ok, err := NewClient(ctx)
	s.Nil(err)
	s.True(ok)
	s.NotNil(client)
}

func (s *CloudClientTestSuite) TestNoCredentialsNoClient() {
	viper.Set(ApiClientIdKey, "")
	viper.Set(ApiClientSecretKey, "")

	ctx := context.Background()
	client, ok, err := NewClient(ctx)
	s.Nil(err)
	s.False(ok)
	s.Nil(client)
}

func (s *CloudClientTestSuite) TestPartialCredentialsReturnError() {
	viper.Set(ApiClientIdKey, "cli_id1234")
	viper.Set(ApiClientSecretKey, "")

	ctx := context.Background()
	_, _, err := NewClient(ctx)
	s.NotNil(err)

	viper.Set(ApiClientIdKey, "")
	viper.Set(ApiClientSecretKey, "secret1234")

	ctx = context.Background()
	_, _, err = NewClient(ctx)
	s.NotNil(err)
}

func TestNewClientTestSuite(t *testing.T) {
	suite.Run(t, &CloudClientTestSuite{})
}
