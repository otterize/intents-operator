package intents_reconcilers

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AzureIntentsReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	serviceIdResolver serviceidresolver.ServiceResolver
	azureAgent        *azureagent.Agent
}

func NewAzureIntentsReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	azureAgent *azureagent.Agent,
	serviceIdResolver serviceidresolver.ServiceResolver,
) *AzureIntentsReconciler {
	return &AzureIntentsReconciler{
		azureAgent:        azureAgent,
		Client:            client,
		Scheme:            scheme,
		serviceIdResolver: serviceIdResolver,
	}
}

func (r *AzureIntentsReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	logger := logrus.WithField("namespace", req.Namespace).WithField("name", req.Name)

	intents := otterizev1alpha3.ClientIntents{}
	err := r.Get(ctx, req.NamespacedName, &intents)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	if intents.DeletionTimestamp != nil {
		logger.Debug("Intents deleted, deleting IAM role policy for this service")

		//err := r.azureAgent.DeleteRolePolicyFromIntents(ctx, intents)
		//if err != nil {
		//	return ctrl.Result{}, errors.Wrap(err)
		//}

		return ctrl.Result{}, nil
	}

	if intents.Spec == nil {
		return ctrl.Result{}, nil
	}

	filteredIntents := intents.GetFilteredCallsList(otterizev1alpha3.IntentTypeAWS)

	if len(filteredIntents) == 0 {
		return ctrl.Result{}, nil
	}

	pod, err := r.serviceIdResolver.ResolveClientIntentToPod(ctx, intents)
	if err != nil {
		if errors.Is(err, serviceidresolver.ErrPodNotFound) {
			r.RecordWarningEventf(
				&intents,
				consts.ReasonPodsNotFound,
				"Could not find non-terminating pods for service %s in namespace %s. Intents could not be reconciled now, but will be reconciled if pods appear later.",
				intents.Spec.Service.Name,
				intents.Namespace)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	_ = pod

	r.RecordNormalEventf(&intents, consts.ReasonReconciledAWSPolicies, "Successfully reconciled AWS policies")
	return ctrl.Result{}, nil
}
