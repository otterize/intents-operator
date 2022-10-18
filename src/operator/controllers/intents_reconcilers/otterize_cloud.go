package intents_reconcilers

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type OtterizeCloudReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	otterizeClientID     string
	otterizeClientSecret string
}

func NewOtterizeCloudReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	otterizeClientID string,
	otterizeClientSecret string) *OtterizeCloudReconciler {

	return &OtterizeCloudReconciler{
		Client:               client,
		Scheme:               scheme,
		otterizeClientID:     otterizeClientID,
		otterizeClientSecret: otterizeClientSecret,
	}
}

func (r *OtterizeCloudReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logrus.Infof("YALLA OTTERIZE CLOUD !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	return ctrl.Result{}, nil
}
