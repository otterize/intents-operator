package controllers

import (
	"context"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodWatcher struct {
	Client client.Client
}

func (r *PodWatcher) Reconcile(ctx context.Context, req ctrl.Request) {

}
