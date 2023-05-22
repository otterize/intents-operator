package reconcilergroup

import (
	"context"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
}

func NewGroup(name string, client client.Client, scheme *runtime.Scheme, reconcilers ...ReconcilerWithEvents) *Group {
	return &Group{reconcilers: reconcilers, name: name, client: client, scheme: scheme}
}

func (g *Group) AddToGroup(reconciler ReconcilerWithEvents) {
	g.reconcilers = append(g.reconcilers, reconciler)
}

func (g *Group) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var finalErr error
	var finalRes ctrl.Result
	logrus.Infof("## Starting reconciliation group cycle for %s", g.name)

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

	return finalRes, finalErr
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
