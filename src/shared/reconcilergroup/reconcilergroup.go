package reconcilergroup

import (
	"context"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ReconcilerWithEvents interface {
	reconcile.Reconciler
	InjectRecorder(recorder record.EventRecorder)
}

type Group struct {
	reconcilers []ReconcilerWithEvents
	name        string
	client      client.Client
	scheme      *runtime.Scheme
	recorder    record.EventRecorder
	baseObject  client.Object
	finalizer   string
}

func NewGroup(
	name string,
	client client.Client,
	scheme *runtime.Scheme,
	resourceObject client.Object,
	finalizer string,
	reconcilers ...ReconcilerWithEvents,
) *Group {
	return &Group{
		reconcilers: reconcilers,
		name:        name,
		client:      client,
		scheme:      scheme,
		baseObject:  resourceObject,
		finalizer:   finalizer,
	}
}

func (g *Group) AddToGroup(reconciler ReconcilerWithEvents) {
	g.reconcilers = append(g.reconcilers, reconciler)
}

func (g *Group) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var finalErr error
	var finalRes ctrl.Result
	logrus.Infof("## Starting reconciliation group cycle for %s", g.name)

	resourceObject := g.baseObject.DeepCopyObject().(client.Object)
	err := g.client.Get(ctx, req.NamespacedName, resourceObject)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}
	if k8serrors.IsNotFound(err) {
		logrus.Infof("Resource %s not found, skipping reconciliation", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	objectBeingDeleted := resourceObject.GetDeletionTimestamp() != nil

	if !objectBeingDeleted {
		err = g.assureFinalizer(ctx, resourceObject)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	finalErr, finalRes = g.runGroup(ctx, req, finalErr, finalRes)
	if objectBeingDeleted && finalErr == nil && finalRes.IsZero() {
		err = g.removeFinalizer(ctx, resourceObject)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return finalRes, finalErr
}

func (g *Group) assureFinalizer(ctx context.Context, resource client.Object) error {
	if !controllerutil.ContainsFinalizer(resource, g.finalizer) {
		controllerutil.AddFinalizer(resource, g.finalizer)
		err := g.client.Update(ctx, resource)
		if err != nil {
			return errors.Wrap(err, "failed to add finalizer")
		}
	}

	return nil
}

func (g *Group) removeFinalizer(ctx context.Context, resource client.Object) error {
	controllerutil.RemoveFinalizer(resource, g.finalizer)
	err := g.client.Update(ctx, resource)
	if err != nil {
		return errors.Wrap(err, "failed to remove finalizer")
	}

	return nil
}

func (g *Group) runGroup(ctx context.Context, req ctrl.Request, finalErr error, finalRes ctrl.Result) (error, ctrl.Result) {
	for _, reconciler := range g.reconcilers {
		logrus.Infof("Starting cycle for %T", reconciler)
		res, err := reconciler.Reconcile(ctx, req)
		if err != nil {
			if finalErr == nil {
				finalErr = err
			}
			logrus.Errorf("Error in reconciler %T: %s", reconciler, err)
		}
		if !res.IsZero() {
			finalRes = shortestRequeue(res, finalRes)
		}
	}
	return finalErr, finalRes
}

func (g *Group) InjectRecorder(recorder record.EventRecorder) {
	g.recorder = recorder
	for _, reconciler := range g.reconcilers {
		reconciler.InjectRecorder(recorder)
	}
}

func shortestRequeue(a, b reconcile.Result) reconcile.Result {
	if a.IsZero() {
		return b
	}
	if b.IsZero() {
		return a
	}
	if a.RequeueAfter < b.RequeueAfter {
		return a
	}
	return b
}
