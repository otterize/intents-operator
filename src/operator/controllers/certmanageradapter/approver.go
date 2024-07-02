package certmanageradapter

import (
	"context"
	apiutil "github.com/cert-manager/cert-manager/pkg/api/util"
	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	utilpki "github.com/cert-manager/cert-manager/pkg/util/pki"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// CertificateApprover - This class is inspired by and shares code with https://github.com/cert-manager/csi-driver-spiffe/tree/main/internal/approver/
type CertificateApprover struct {
	client    client.Client
	lister    client.Reader
	issuerRef cmmeta.ObjectReference
	registry  *CertManagerWorkloadRegistry
}

func NewCertificateApprover(issuerRef cmmeta.ObjectReference, client client.Client, lister client.Reader, registry *CertManagerWorkloadRegistry) *CertificateApprover {
	return &CertificateApprover{
		issuerRef: issuerRef,
		client:    client,
		lister:    lister,
		registry:  registry,
	}
}

func (a *CertificateApprover) Register(ctx context.Context, mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		For(new(cmapi.CertificateRequest)).
		Complete(a)
}

// Reconcile is called when a CertificateRequest is synced which has been
// neither approved or denied yet, and matches the issuerRef configured.
func (a *CertificateApprover) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logrus.WithFields(logrus.Fields{"namespace": req.NamespacedName.Namespace, "name": req.NamespacedName.Name})

	var certReq cmapi.CertificateRequest
	if err := a.lister.Get(ctx, req.NamespacedName, &certReq); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ignore requests that already have an Approved or Denied condition.
	if apiutil.CertificateRequestIsApproved(&certReq) || apiutil.CertificateRequestIsDenied(&certReq) || certReq.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	if certReq.Spec.IssuerRef != a.issuerRef {
		return ctrl.Result{}, nil
	}

	log.Info("syncing certificaterequest")
	defer log.Info("finished syncing certificaterequest")

	if err := a.Evaluate(&certReq); err != nil {
		log.Error("denying request: ", err)
		apiutil.SetCertificateRequestCondition(&certReq, cmapi.CertificateRequestConditionDenied, cmmeta.ConditionTrue, "credentials-operator.otterize.com", "Denied request: "+err.Error())
		return ctrl.Result{}, a.client.Status().Update(ctx, &certReq)
	}

	log.Info("approving request")
	apiutil.SetCertificateRequestCondition(&certReq, cmapi.CertificateRequestConditionApproved, cmmeta.ConditionTrue, "credentials-operator.otterize.com", "Approved request")
	return ctrl.Result{}, a.client.Status().Update(ctx, &certReq)
}

// Evaluate evaluates whether a CertificateRequest should be approved or
// denied. A CertificateRequest should be denied if this function returns an
// error, should be approved otherwise.
func (a *CertificateApprover) Evaluate(req *cmapi.CertificateRequest) error {
	csr, err := utilpki.DecodeX509CertificateRequestBytes(req.Spec.Request)
	if err != nil {
		return errors.Errorf("failed to parse request: %w", err)
	}

	if err := csr.CheckSignature(); err != nil {
		return errors.Errorf("signature check failed for csr: %w", err)
	}

	// if the csr contains any other options set, error
	if len(csr.IPAddresses) > 0 || len(csr.EmailAddresses) > 0 {
		return errors.Errorf("forbidden extensions, IPs=%q Emails=%q",
			csr.IPAddresses, csr.EmailAddresses)
	}

	if req.Spec.IsCA {
		return errors.Errorf("request contains spec.isCA=true")
	}

	entryId, ok := req.Annotations[metadata.TLSSecretEntryIDAnnotation]
	if !ok {
		return errors.Errorf("credentials-operator's annotation not found")
	}

	if a.registry.getPodEntryById(entryId) == nil {
		return errors.Errorf("entry-id does not exist: %q", entryId)
	}

	return nil
}
