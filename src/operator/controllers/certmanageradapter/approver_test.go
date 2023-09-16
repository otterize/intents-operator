package certmanageradapter

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/pem"
	"fmt"
	apiutil "github.com/cert-manager/cert-manager/pkg/api/util"
	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	utilpki "github.com/cert-manager/cert-manager/pkg/util/pki"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"testing"
	"time"
)

type CertApproverSuite struct {
	suite.Suite
	controller *gomock.Controller
	registry   *CertManagerWorkloadRegistry
	pk         *ecdsa.PrivateKey
}

func (s *CertApproverSuite) SetupTest() {
	s.controller = gomock.NewController(s.T())

	s.registry = NewCertManagerWorkloadRegistry()

	var err error
	s.pk, err = utilpki.GenerateECPrivateKey(utilpki.ECCurve521)
	s.Require().NoError(err, "failed generating private key")

}

func (s *CertApproverSuite) CheckReconcile(cr *cmapi.CertificateRequest, valid bool) {
	certReqName := types.NamespacedName{Namespace: "test-ns", Name: "test-cr"}
	cr.SetNamespace(certReqName.Namespace)
	cr.SetName(certReqName.Name)
	cr.Spec.IssuerRef = cmmeta.ObjectReference{
		Name: "my-issuer",
		Kind: "ClusterIssuer",
	}
	objs := []client.Object{cr}

	scheme := runtime.NewScheme()
	s.Require().NoError(clientgoscheme.AddToScheme(scheme))
	s.Require().NoError(cmapi.AddToScheme(scheme))

	fakeClient := fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).WithStatusSubresource(objs...).Build()

	approver := NewCertificateApprover(cmmeta.ObjectReference{
		Kind: "ClusterIssuer",
		Name: "my-issuer",
	}, fakeClient, fakeClient, s.registry)

	_, err := approver.Reconcile(context.TODO(), ctrl.Request{NamespacedName: certReqName})
	s.Require().NoError(err)

	updatedCr := &cmapi.CertificateRequest{}
	s.Require().NoError(fakeClient.Get(context.TODO(), client.ObjectKeyFromObject(cr), updatedCr))
	if valid {
		s.Require().True(apiutil.CertificateRequestIsApproved(updatedCr), "CertificateRequest wasn't approved")
	} else {
		s.Require().True(apiutil.CertificateRequestIsDenied(updatedCr), "CertificateRequest wasn't denied")
	}
}

func (s *CertApproverSuite) Test_ReconcileDeny_CSRFormat() {
	entryId, err := s.registry.RegisterK8SPod(context.TODO(), "test-ns", "", "test-pod", 99999, []string{})
	s.Require().NoError(err, "failed registering pod")

	s.CheckReconcile(&cmapi.CertificateRequest{
		Spec: cmapi.CertificateRequestSpec{
			Request:  []byte("bad-pem"),
			Duration: &metav1.Duration{Duration: time.Hour},
			Usages: []cmapi.KeyUsage{
				cmapi.UsageServerAuth, cmapi.UsageClientAuth,
				cmapi.UsageDigitalSignature, cmapi.UsageKeyEncipherment,
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{metadata.TLSSecretEntryIDAnnotation: entryId},
		}}, false)
}

func (s *CertApproverSuite) Test_ReconcileDeny_EmailAddresses() {
	entryId, err := s.registry.RegisterK8SPod(context.TODO(), "test-ns", "", "test-pod", 99999, []string{})
	s.Require().NoError(err, "failed registering pod")

	csr, err := utilpki.GenerateCSR(&cmapi.Certificate{
		Spec: cmapi.CertificateSpec{
			PrivateKey:     &cmapi.CertificatePrivateKey{Algorithm: cmapi.ECDSAKeyAlgorithm},
			CommonName:     "test-name",
			DNSNames:       []string{"test-name.com", "www.test-name.com"},
			EmailAddresses: []string{"dontreply@test.com"},
		},
	})
	s.Require().NoError(err, "failed generating CSR")
	csrDER, err := utilpki.EncodeCSR(csr, s.pk)
	s.Require().NoError(err, "failed encoding CSR")
	csrPEM := bytes.NewBuffer([]byte{})
	s.Require().NoError(pem.Encode(csrPEM, &pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))

	s.CheckReconcile(&cmapi.CertificateRequest{
		Spec: cmapi.CertificateRequestSpec{
			Request:  csrPEM.Bytes(),
			Duration: &metav1.Duration{Duration: time.Hour},
			Usages: []cmapi.KeyUsage{
				cmapi.UsageServerAuth, cmapi.UsageClientAuth,
				cmapi.UsageDigitalSignature, cmapi.UsageKeyEncipherment,
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{metadata.TLSSecretEntryIDAnnotation: entryId},
		}}, false)
}

func (s *CertApproverSuite) Test_ReconcileDeny_CA() {
	entryId, err := s.registry.RegisterK8SPod(context.TODO(), "test-ns", "", "test-pod", 99999, []string{})
	s.Require().NoError(err, "failed registering pod")

	csr, err := utilpki.GenerateCSR(&cmapi.Certificate{
		Spec: cmapi.CertificateSpec{
			PrivateKey: &cmapi.CertificatePrivateKey{Algorithm: cmapi.ECDSAKeyAlgorithm},
			CommonName: "test-name",
			DNSNames:   []string{"test-name.com", "www.test-name.com"},
		},
	})
	s.Require().NoError(err, "failed generating CSR")
	csrDER, err := utilpki.EncodeCSR(csr, s.pk)
	s.Require().NoError(err, "failed encoding CSR")
	csrPEM := bytes.NewBuffer([]byte{})
	s.Require().NoError(pem.Encode(csrPEM, &pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))

	s.CheckReconcile(&cmapi.CertificateRequest{
		Spec: cmapi.CertificateRequestSpec{
			IsCA:     true,
			Request:  csrPEM.Bytes(),
			Duration: &metav1.Duration{Duration: time.Hour},
			Usages: []cmapi.KeyUsage{
				cmapi.UsageServerAuth, cmapi.UsageClientAuth,
				cmapi.UsageDigitalSignature, cmapi.UsageKeyEncipherment,
				cmapi.UsageSigning,
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{metadata.TLSSecretEntryIDAnnotation: entryId},
		}}, false)
}
func (s *CertApproverSuite) Test_ReconcileDeny_NoAnnotation() {
	csr, err := utilpki.GenerateCSR(&cmapi.Certificate{
		Spec: cmapi.CertificateSpec{
			PrivateKey: &cmapi.CertificatePrivateKey{Algorithm: cmapi.ECDSAKeyAlgorithm},
			CommonName: "test-name",
			DNSNames:   []string{"test-name.com", "www.test-name.com"},
		},
	})
	s.Require().NoError(err, "failed generating CSR")
	csrDER, err := utilpki.EncodeCSR(csr, s.pk)
	s.Require().NoError(err, "failed encoding CSR")
	csrPEM := bytes.NewBuffer([]byte{})
	s.Require().NoError(pem.Encode(csrPEM, &pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))

	s.CheckReconcile(&cmapi.CertificateRequest{
		Spec: cmapi.CertificateRequestSpec{
			Request:  csrPEM.Bytes(),
			Duration: &metav1.Duration{Duration: time.Hour},
			Usages: []cmapi.KeyUsage{
				cmapi.UsageServerAuth, cmapi.UsageClientAuth,
				cmapi.UsageDigitalSignature, cmapi.UsageKeyEncipherment,
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{},
		}}, false)
}
func (s *CertApproverSuite) Test_ReconcileDeny_NonExistingEntry() {
	entryId, err := s.registry.RegisterK8SPod(context.TODO(), "test-ns", "", "test-pod", 99999, []string{})
	s.Require().NoError(err, "failed registering pod")

	csr, err := utilpki.GenerateCSR(&cmapi.Certificate{
		Spec: cmapi.CertificateSpec{
			PrivateKey: &cmapi.CertificatePrivateKey{Algorithm: cmapi.ECDSAKeyAlgorithm},
			CommonName: "test-name",
			DNSNames:   []string{"test-name.com", "www.test-name.com"},
		},
	})
	s.Require().NoError(err, "failed generating CSR")
	csrDER, err := utilpki.EncodeCSR(csr, s.pk)
	s.Require().NoError(err, "failed encoding CSR")
	csrPEM := bytes.NewBuffer([]byte{})
	s.Require().NoError(pem.Encode(csrPEM, &pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))

	s.CheckReconcile(&cmapi.CertificateRequest{
		Spec: cmapi.CertificateRequestSpec{
			Request:  csrPEM.Bytes(),
			Duration: &metav1.Duration{Duration: time.Hour},
			Usages: []cmapi.KeyUsage{
				cmapi.UsageServerAuth, cmapi.UsageClientAuth,
				cmapi.UsageDigitalSignature, cmapi.UsageKeyEncipherment,
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{metadata.TLSSecretEntryIDAnnotation: fmt.Sprintf("not-%s", entryId)},
		}}, false)
}

func (s *CertApproverSuite) Test_ReconcileApprove() {
	entryId, err := s.registry.RegisterK8SPod(context.TODO(), "test-ns", "", "test-pod", 99999, []string{})
	s.Require().NoError(err, "failed registering pod")

	csr, err := utilpki.GenerateCSR(&cmapi.Certificate{
		Spec: cmapi.CertificateSpec{
			PrivateKey: &cmapi.CertificatePrivateKey{Algorithm: cmapi.ECDSAKeyAlgorithm},
			CommonName: "test-name",
			DNSNames:   []string{"test-name.com", "www.test-name.com"},
		},
	})
	s.Require().NoError(err, "failed generating CSR")
	csrDER, err := utilpki.EncodeCSR(csr, s.pk)
	s.Require().NoError(err, "failed encoding CSR")
	csrPEM := bytes.NewBuffer([]byte{})
	s.Require().NoError(pem.Encode(csrPEM, &pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))

	s.CheckReconcile(&cmapi.CertificateRequest{
		Spec: cmapi.CertificateRequestSpec{
			Request:  csrPEM.Bytes(),
			Duration: &metav1.Duration{Duration: time.Hour},
			Usages: []cmapi.KeyUsage{
				cmapi.UsageServerAuth, cmapi.UsageClientAuth,
				cmapi.UsageDigitalSignature, cmapi.UsageKeyEncipherment,
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{metadata.TLSSecretEntryIDAnnotation: entryId},
		}}, true)
}

func TestCertApproverSuite(t *testing.T) {
	suite.Run(t, new(CertApproverSuite))
}
