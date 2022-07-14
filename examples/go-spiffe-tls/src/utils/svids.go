package utils

import (
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
)

type LocalSVIDSource struct {
	certFilePath string
	keyFilePath  string
}

func NewLocalSVIDSource(certFilePath string, keyFilePath string) *LocalSVIDSource {
	return &LocalSVIDSource{certFilePath: certFilePath, keyFilePath: keyFilePath}
}

func (s LocalSVIDSource) GetX509SVID() (*x509svid.SVID, error) {
	svid, err := x509svid.Load(s.certFilePath, s.keyFilePath)
	if err != nil {
		logrus.WithError(err).Error("error loading svid")
		return nil, err
	}
	logrus.WithField("spiffeid", svid.ID.String()).Info("SVID Loaded")
	return svid, nil
}
