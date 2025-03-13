package intents_reconcilers

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type UpToDateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	intentsCounter      map[string]int
	typedIntentsCounter map[string]map[otterizev2alpha1.IntentType]int
}

func NewUpToDateReconciler(client client.Client, scheme *runtime.Scheme) *UpToDateReconciler {
	return &UpToDateReconciler{
		Client:              client,
		Scheme:              scheme,
		intentsCounter:      make(map[string]int),
		typedIntentsCounter: make(map[string]map[otterizev2alpha1.IntentType]int),
	}
}

// Reconcile compares the UpToDate status of ApprovedClientIntents and ClientIntents and updates the ClientIntents status if necessary
func (r *UpToDateReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	approvedIntents := &otterizev2alpha1.ApprovedClientIntents{}
	err := r.Get(ctx, req.NamespacedName, approvedIntents)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	// Get corresponding ClientIntents
	clientIntents := &otterizev2alpha1.ClientIntents{}
	err = r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: approvedIntents.ToClientIntentsName()}, clientIntents)
	if k8serrors.IsNotFound(err) {
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	// Compare UpToDate
	if approvedIntents.Status.UpToDate == clientIntents.Status.UpToDate {
		return ctrl.Result{}, nil
	}

	clientIntentsPatch := clientIntents.DeepCopy()
	clientIntentsPatch.Status.UpToDate = approvedIntents.Status.UpToDate
	err = r.Status().Patch(ctx, clientIntentsPatch, client.MergeFrom(clientIntents))
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *UpToDateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&otterizev2alpha1.ApprovedClientIntents{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)
}
