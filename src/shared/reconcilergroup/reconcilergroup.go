package reconcilergroup

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/errors"
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
	reconcilers      []ReconcilerWithEvents
	name             string
	client           client.Client
	scheme           *runtime.Scheme
	recorder         record.EventRecorder
	baseObject       client.Object
	finalizer        string
	legacyFinalizers []string
}

func NewGroup(
	name string,
	client client.Client,
	scheme *runtime.Scheme,
	resourceObject client.Object,
	finalizer string,
	legacyFinalizers []string,
	reconcilers ...ReconcilerWithEvents,
) *Group {
	return &Group{
		reconcilers:      reconcilers,
		name:             name,
		client:           client,
		scheme:           scheme,
		baseObject:       resourceObject,
		finalizer:        finalizer,
		legacyFinalizers: legacyFinalizers,
	}
}

func (g *Group) AddToGroup(reconciler ReconcilerWithEvents) {
	g.reconcilers = append(g.reconcilers, reconciler)
}

func (g *Group) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var finalErr error
	var finalRes ctrl.Result
	logrus.Debugf("## Starting reconciliation group cycle for %s", g.name)

	resourceObject := g.baseObject.DeepCopyObject().(client.Object)
	err := g.client.Get(ctx, req.NamespacedName, resourceObject)
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}
	if k8serrors.IsNotFound(err) {
		logrus.Debugf("Resource %s not found, skipping reconciliation", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	err = g.ensureFinalizer(ctx, resourceObject)
	if err != nil {
		if isKubernetesRaceRelatedError(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	err = g.removeLegacyFinalizers(ctx, resourceObject)
	if err != nil {
		if isKubernetesRaceRelatedError(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	finalRes, finalErr = g.runGroup(ctx, req, finalErr, finalRes)

	objectBeingDeleted := resourceObject.GetDeletionTimestamp() != nil
	if objectBeingDeleted && finalErr == nil && finalRes.IsZero() {
		err = g.removeFinalizer(ctx, resourceObject)
		if err != nil {
			if isKubernetesRaceRelatedError(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, errors.Wrap(err)
		}
	}

	return finalRes, finalErr
}

func (g *Group) removeLegacyFinalizers(ctx context.Context, resource client.Object) error {
	shouldUpdate := false
	for _, legacyFinalizer := range g.legacyFinalizers {
		if controllerutil.ContainsFinalizer(resource, legacyFinalizer) {
			controllerutil.RemoveFinalizer(resource, legacyFinalizer)
			shouldUpdate = true
		}
	}

	if shouldUpdate {
		err := g.client.Update(ctx, resource)
		if err != nil {
			return errors.Errorf("failed to remove legacy finalizers: %w", err)
		}
	}

	return nil
}

func (g *Group) ensureFinalizer(ctx context.Context, resource client.Object) error {
	if !controllerutil.ContainsFinalizer(resource, g.finalizer) {
		controllerutil.AddFinalizer(resource, g.finalizer)
		err := g.client.Update(ctx, resource)
		if err != nil {
			return errors.Errorf("failed to add finalizer: %w", err)
		}
	}

	return nil
}

func (g *Group) removeFinalizer(ctx context.Context, resource client.Object) error {
	controllerutil.RemoveFinalizer(resource, g.finalizer)
	err := g.client.Update(ctx, resource)
	if err != nil {
		return errors.Errorf("failed to remove finalizer: %w", err)
	}

	return nil
}

func (g *Group) runGroup(ctx context.Context, req ctrl.Request, finalErr error, finalRes ctrl.Result) (ctrl.Result, error) {
	for _, reconciler := range g.reconcilers {
		logrus.Debugf("Starting cycle for %T", reconciler)
		res, err := reconciler.Reconcile(ctx, req)
		if err != nil {
			if finalErr == nil {
				finalErr = err
			}
		}
		if !res.IsZero() {
			finalRes = shortestRequeue(res, finalRes)
		}
	}
	return finalRes, finalErr
}

func (g *Group) InjectRecorder(recorder record.EventRecorder) {
	g.recorder = recorder
	for _, reconciler := range g.reconcilers {
		reconciler.InjectRecorder(recorder)
	}
}

func isKubernetesRaceRelatedError(err error) bool {
	errUnwrap := errors.Unwrap(err)
	return k8serrors.IsConflict(errUnwrap) || k8serrors.IsNotFound(errUnwrap) || k8serrors.IsForbidden(errUnwrap) || k8serrors.IsAlreadyExists(errUnwrap)
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
