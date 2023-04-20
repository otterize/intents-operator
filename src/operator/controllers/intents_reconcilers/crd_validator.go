package intents_reconcilers

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	IntentsCRDName       = "clientintents.k8s.otterize.com"
	CRDVersionToValidate = "v1alpha2"
)

type CRDValidatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	injectablerecorder.InjectableRecorder
	validatedCRD bool
}

func NewCRDValidatorReconciler(c client.Client, s *runtime.Scheme) *CRDValidatorReconciler {
	return &CRDValidatorReconciler{
		Client:       c,
		Scheme:       s,
		validatedCRD: false,
	}
}

func (r *CRDValidatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.validatedCRD {
		return ctrl.Result{}, nil
	}

	intentsCRD := v1.CustomResourceDefinition{}
	err := r.Get(ctx, types.NamespacedName{Name: IntentsCRDName}, &intentsCRD)
	if err != nil {
		logrus.WithError(err).Error("failed validating intents CRD")
		return ctrl.Result{}, nil
	}
	if intentsCRD.Spec.Versions[0].Name != CRDVersionToValidate {
		return ctrl.Result{}, nil
	}

	// God, please forgive us
	requiredResources := intentsCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["calls"].
		Items.Schema.Properties["resources"].Items.Schema.Required
	if len(requiredResources) != 1 {
		if slices.Contains(requiredResources, "methods") {
			intentsCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"].Properties["calls"].
				Items.Schema.Properties["resources"].Items.Schema.Required = []string{"path"}
		}
		if err = r.Update(ctx, &intentsCRD); err != nil {
			logrus.WithError(err).Error("failed validating intents CRD")
			return ctrl.Result{}, nil
		}
		r.validatedCRD = true
	}

	return ctrl.Result{}, nil
}
