package iam_pod_reconciler

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	OtterizeClientNameIndexField = "spec.service.name"
)

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;update;patch;list;watch

type IAMPodReconciler struct {
	client.Client
	serviceIdResolver *serviceidresolver.Resolver
	injectablerecorder.InjectableRecorder
	iamReconciler reconcile.Reconciler
}

func NewIAMPodReconciler(c client.Client, eventRecorder record.EventRecorder, iamIntentsReconciler reconcile.Reconciler) *IAMPodReconciler {
	recorder := injectablerecorder.InjectableRecorder{Recorder: eventRecorder}
	return &IAMPodReconciler{
		Client:             c,
		serviceIdResolver:  serviceidresolver.NewResolver(c),
		InjectableRecorder: recorder,
		iamReconciler:      iamIntentsReconciler,
	}
}

func (p *IAMPodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logrus.WithField("namespace", req.Namespace).WithField("name", req.Name)
	logger.Infof("Reconciling due to pod change")

	pod := v1.Pod{}
	err := p.Get(ctx, req.NamespacedName, &pod)

	if k8serrors.IsNotFound(err) || pod.DeletionTimestamp != nil {
		logger.Infoln("Pod was deleted")
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	serviceID, err := p.serviceIdResolver.ResolvePodToServiceIdentity(ctx, &pod)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	// If a new pod starts, check if we need to do something for it.
	var intents otterizev1alpha3.ClientIntentsList
	err = p.List(
		ctx,
		&intents,
		client.MatchingFields{OtterizeClientNameIndexField: serviceID.Name},
		&client.ListOptions{Namespace: pod.Namespace})
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	for _, intent := range intents.Items {
		return p.iamReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      intent.Name,
			Namespace: intent.Namespace,
		}})
	}

	return ctrl.Result{}, nil
}

func (p *IAMPodReconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Pod{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(p)
}
