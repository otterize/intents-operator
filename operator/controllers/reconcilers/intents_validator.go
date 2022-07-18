package reconcilers

import (
	"context"
	"errors"
	otterizev1alpha1 "github.com/otterize/otternose/api/v1alpha1"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type IntentsValidatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *IntentsValidatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	intents := &otterizev1alpha1.Intents{}
	err := r.Get(ctx, req.NamespacedName, intents)
	if k8serrors.IsNotFound(err) {
		logrus.Infof("No intents found for namespace %s\n", req.NamespacedName.Namespace)
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	if intents.Spec == nil {
		// No intents for namespace, move on
		return ctrl.Result{}, nil
	}

	serviceName := intents.GetServiceName()
	logrus.Debugln("Intents for service: " + serviceName)
	for _, intent := range intents.GetCallList() {
		logrus.Debugf("%s has intent to access %s. Intent type: %s\n", serviceName, intent.Server, intent.Type)
		if err := validateIntent(intent); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func validateIntent(intent otterizev1alpha1.Intent) error {
	if intent.Type == otterizev1alpha1.IntentTypeKafka {
		if intent.HTTPResources != nil {
			return errors.New("invalid intent format. type 'Kafka' cannot contain HTTP resources")
		}
	}

	if intent.Type == otterizev1alpha1.IntentTypeHTTP {
		if intent.Topics != nil {
			return errors.New("invalid intent format. type 'HTTP' cannot contain kafka topics")
		}
	}

	return nil
}
