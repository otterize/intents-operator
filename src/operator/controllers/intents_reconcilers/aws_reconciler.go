package intents_reconcilers

import (
	"context"
	"errors"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
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
		logger.Infof("Intents deleted")

		err := r.awsAgent.DeleteRolePolicy(ctx, req.Namespace, req.Name)

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
			// TODO: fix pod watcher logic to handle this case when pod starts later
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if pod.Annotations == nil {
		return ctrl.Result{}, nil
	}

	serviceAccountName, found := pod.Annotations["credentials-operator.otterize.com/service-account-name"]

	if !found {
		r.RecordWarningEventf(&intents, consts.ReasonAWSIntentsFoundButNoServiceAccount, "Found AWS intents, but no service account annotation specified for this pod ('%s').", pod.Name)
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

	err = r.awsAgent.AddRolePolicy(ctx, req.Namespace, serviceAccountName, req.Name, policy.Statement)

	return ctrl.Result{}, err
}
