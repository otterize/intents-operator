package intents_reconcilers

import (
	"context"
	"errors"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AWSIntentsReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	serviceIdResolver serviceidresolver.ServiceResolver
	awsAgent          *awsagent.Agent
}

func NewAWSIntentsReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	awsAgent *awsagent.Agent,
	serviceIdResolver serviceidresolver.ServiceResolver,
) *AWSIntentsReconciler {
	return &AWSIntentsReconciler{
		awsAgent:          awsAgent,
		Client:            client,
		Scheme:            scheme,
		serviceIdResolver: serviceIdResolver,
	}
}

func (r *AWSIntentsReconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	logger := logrus.WithField("namespace", req.Namespace).WithField("name", req.Name)

	intents := otterizev1alpha3.ClientIntents{}
	err := r.Get(ctx, req.NamespacedName, &intents)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if intents.DeletionTimestamp != nil {
		logger.Debug("Intents deleted, deleting IAM role policy for this service")

		err := r.awsAgent.DeleteRolePolicyFromIntents(ctx, intents)
		if err != nil {
			return ctrl.Result{}, err
		}

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
		return ctrl.Result{}, err
	}

	if pod.Labels == nil {
		return ctrl.Result{}, nil
	}

	if _, ok := pod.Labels["credentials-operator.otterize.com/create-aws-role"]; !ok {
		return ctrl.Result{}, nil
	}

	serviceAccountName := pod.Spec.ServiceAccountName

	hasMultipleClientsForServiceAccount, err := r.hasMultipleClientsForServiceAccount(ctx, serviceAccountName, pod.Namespace)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed checking if the service account: %s is used by multiple aws clients: %w", serviceAccountName, err)
	}

	if hasMultipleClientsForServiceAccount {
		r.RecordWarningEventf(&intents, consts.ReasonAWSIntentsServiceAccountUsedByMultipleClients, "found multiple clients using the service account: %s", serviceAccountName)
		return ctrl.Result{}, nil
	}

	if len(serviceAccountName) == 0 {
		r.RecordWarningEventf(&intents, consts.ReasonAWSIntentsFoundButNoServiceAccount, "Found AWS intents, but no service account found for pod ('%s').", pod.Name)
		return ctrl.Result{}, nil
	}

	policy := awsagent.PolicyDocument{
		Version: "2012-10-17",
	}

	for _, intent := range filteredIntents {
		awsResource := intent.Name
		actions := intent.AWSActions

		policy.Statement = append(policy.Statement, awsagent.StatementEntry{
			Effect:   "Allow",
			Resource: awsResource,
			Action:   actions,
		})
	}

	err = r.awsAgent.AddRolePolicy(ctx, req.Namespace, serviceAccountName, intents.Spec.Service.Name, policy.Statement)
	return ctrl.Result{}, err
}

func (r *AWSIntentsReconciler) hasMultipleClientsForServiceAccount(ctx context.Context, serviceAccountName string, namespace string) (bool, error) {
	var intents otterizev1alpha3.ClientIntentsList
	err := r.List(
		ctx,
		&intents,
		&client.ListOptions{Namespace: namespace})
	if err != nil {
		return false, err
	}

	if len(intents.Items) <= 1 {
		return false, nil
	}

	intentsWithAWSInSameNamespace := lo.Filter(intents.Items, func(intent otterizev1alpha3.ClientIntents, _ int) bool {
		return len(intent.GetFilteredCallsList(otterizev1alpha3.IntentTypeAWS)) != 0
	})

	countUsesOfServiceAccountName := 0
	for _, intent := range intentsWithAWSInSameNamespace {
		pod, err := r.serviceIdResolver.ResolveClientIntentToPod(ctx, intent)
		if err != nil {
			return false, err
		}
		if pod.Spec.ServiceAccountName == serviceAccountName {
			countUsesOfServiceAccountName++
		}
	}

	return countUsesOfServiceAccountName > 1, nil

}
