package iam

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/iam/iampolicyagents"
	"github.com/otterize/intents-operator/src/shared/agentutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type IAMIntentsReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	serviceIdResolver serviceidresolver.ServiceResolver
	agent             iampolicyagents.IAMPolicyAgent
}

func NewIAMIntentsReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	serviceIdResolver serviceidresolver.ServiceResolver,
	agent iampolicyagents.IAMPolicyAgent,
) *IAMIntentsReconciler {
	return &IAMIntentsReconciler{
		Client:            client,
		Scheme:            scheme,
		serviceIdResolver: serviceIdResolver,
		agent:             agent,
	}
}

func (r *IAMIntentsReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	logger := logrus.WithField("namespace", req.Namespace).WithField("name", req.Name)

	intents := otterizev2alpha1.ApprovedClientIntents{}
	err := r.Get(ctx, req.NamespacedName, &intents)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	if intents.DeletionTimestamp != nil {
		logger.Debug("Intents deleted, deleting IAM role policy for this service")

		err := r.agent.DeleteRolePolicyFromIntents(ctx, intents)
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}

		return ctrl.Result{}, nil
	}

	if intents.Spec == nil {
		return ctrl.Result{}, nil
	}

	pod, err := r.serviceIdResolver.ResolveClientIntentToPod(ctx, intents)
	if err != nil {
		if errors.Is(err, serviceidresolver.ErrPodNotFound) {
			r.RecordWarningEventf(
				&intents,
				consts.ReasonPodsNotFound,
				"Could not find non-terminating pods for service %s in namespace %s. Intents could not be reconciled now, but will be reconciled if pods appear later.",
				intents.Spec.Workload.Name,
				intents.Namespace)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	if err := r.applyTypedIAMIntents(ctx, pod, intents, r.agent); err != nil {
		if errors.Is(err, agentutils.ErrRoleNotFound) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	return ctrl.Result{}, nil
}

func (r *IAMIntentsReconciler) applyTypedIAMIntents(ctx context.Context, pod corev1.Pod, intents otterizev2alpha1.ApprovedClientIntents, agent iampolicyagents.IAMPolicyAgent) error {
	if !agent.AppliesOnPod(&pod) {
		return nil
	}

	intentType := agent.IntentType()

	serviceAccountName := pod.Spec.ServiceAccountName
	if serviceAccountName == "" {
		r.RecordWarningEventf(&intents, consts.ReasonIntentsFoundButNoServiceAccount, "Found IAM intents, but no service account found for pod ('%s').", pod.Name)
		return nil
	}

	hasMultipleClientsForServiceAccount, err := r.hasMultipleClientsForServiceAccount(ctx, serviceAccountName, pod.Namespace, intentType)
	if err != nil {
		return errors.Errorf("failed checking if the service account: %s is used by multiple IAM clients of type %s: %w", serviceAccountName, intentType, err)
	}

	if hasMultipleClientsForServiceAccount {
		r.RecordWarningEventf(&intents, consts.ReasonIntentsServiceAccountUsedByMultipleClients, "found multiple clients of type %s using the service account: %s", intentType, serviceAccountName)
		return nil
	}

	filteredIntents := intents.GetFilteredTargetList(intentType)
	err = agent.AddRolePolicyFromIntents(ctx, pod.Namespace, serviceAccountName, intents.Spec.Workload.Name, filteredIntents, pod)
	if err != nil {
		r.RecordWarningEventf(&intents, consts.ReasonReconcilingIAMPoliciesFailed, "Failed to reconcile IAM policies of type %s due to error: %s", intentType, err.Error())
		return errors.Wrap(err)
	}

	r.RecordNormalEventf(&intents, consts.ReasonReconciledIAMPolicies, "Successfully reconciled IAM policies of type %s", intentType)
	return nil
}

func (r *IAMIntentsReconciler) hasMultipleClientsForServiceAccount(ctx context.Context, serviceAccountName string, namespace string, intentType otterizev2alpha1.IntentType) (bool, error) {
	var intents otterizev2alpha1.ApprovedClientIntentsList
	err := r.List(ctx, &intents, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return false, errors.Wrap(err)
	}

	if len(intents.Items) <= 1 {
		return false, nil
	}

	intentsWithSameTypeInSameNamespace := lo.Filter(intents.Items, func(intent otterizev2alpha1.ApprovedClientIntents, _ int) bool {
		return len(intent.GetFilteredTargetList(intentType)) != 0
	})

	countUsesOfServiceAccountName := 0
	for _, intent := range intentsWithSameTypeInSameNamespace {
		pod, err := r.serviceIdResolver.ResolveClientIntentToPod(ctx, intent)
		if errors.Is(err, serviceidresolver.ErrPodNotFound) {
			continue
		}
		if err != nil {
			return false, errors.Wrap(err)
		}
		if pod.Spec.ServiceAccountName == serviceAccountName {
			countUsesOfServiceAccountName++
		}
	}

	return countUsesOfServiceAccountName > 1, nil

}
