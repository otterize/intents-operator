package protected_service_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ReconcilerFunc func(ctx context.Context, req ctrl.Request) error

func WithFinalizer(ctx context.Context, client client.Client, req ctrl.Request, finalizerName string, callBack ReconcilerFunc) error {
	var protectedService otterizev1alpha2.ProtectedService
	err := client.Get(ctx, req.NamespacedName, &protectedService)
	if err != nil {
		return err
	}

	if protectedService.DeletionTimestamp != nil {
		return doAndRemoveFinalizer(ctx, client, req, finalizerName, callBack, err, protectedService)
	}

	return addFinalizerAndDo(ctx, client, req, finalizerName, callBack, protectedService, err)
}

func addFinalizerAndDo(ctx context.Context, client client.Client, req ctrl.Request, finalizerName string, callBack ReconcilerFunc, protectedService otterizev1alpha2.ProtectedService, err error) error {
	if !controllerutil.ContainsFinalizer(&protectedService, finalizerName) {
		controllerutil.AddFinalizer(&protectedService, finalizerName)
		err = client.Update(ctx, &protectedService)
		if err != nil {
			return err
		}
	}

	return callBack(ctx, req)
}

func doAndRemoveFinalizer(ctx context.Context, client client.Client, req ctrl.Request, finalizerName string, callBack ReconcilerFunc, err error, protectedService otterizev1alpha2.ProtectedService) error {
	err = callBack(ctx, req)
	if err != nil {
		return err
	}

	controllerutil.RemoveFinalizer(&protectedService, finalizerName)
	return client.Update(ctx, &protectedService)
}
