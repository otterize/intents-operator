package utils

import (
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
)

type LocalBundleSource struct {
	path string
}

func NewLocalBundleSource(path string) *LocalBundleSource {
	return &LocalBundleSource{path: path}
}

func (s LocalBundleSource) GetX509BundleForTrustDomain(trustDomain spiffeid.TrustDomain) (*x509bundle.Bundle, error) {
	bundle, err := x509bundle.Load(trustDomain, s.path)
	if err != nil {
		logrus.Error(err, "error loading bundle")
		return nil, err
	}

	logrus.WithField("trust_domain", bundle.TrustDomain().String()).Info("bundle loaded")
	return bundle, nil
}
