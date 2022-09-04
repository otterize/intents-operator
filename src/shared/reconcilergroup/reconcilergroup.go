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

func (g *Group) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logrus.Infof("## Starting reconciliation group cycle for %s", g.name)
	for _, reconciler := range g.reconcilers {
		logrus.Infof("Starting cycle for %T", reconciler)
		res, err := reconciler.Reconcile(ctx, req)
		if res.Requeue == true || err != nil {
			return res, err
		}
	}

	return ctrl.Result{}, nil
}

func (g *Group) InjectRecorder(recorder record.EventRecorder) {
	g.recorder = recorder
	for _, reconciler := range g.reconcilers {
		reconciler.InjectRecorder(recorder)
	}
}
