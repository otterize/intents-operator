package reconcilergroup

import (
	"context"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Group struct {
	reconcilers []reconcile.Reconciler
	name        string
	client      client.Client
	scheme      *runtime.Scheme
}

func NewGroup(name string, client client.Client, scheme *runtime.Scheme, reconcilers ...reconcile.Reconciler) *Group {
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
