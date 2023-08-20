package protected_service_reconcilers

import (
	"context"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func HandleFinalizer(ctx context.Context, client client.Client, req ctrl.Request, finalizerName string) error {
	var protectedService otterizev1alpha2.ProtectedService
	err := client.Get(ctx, req.NamespacedName, &protectedService)
	if err != nil {
		return err
	}

	if protectedService.DeletionTimestamp != nil {
		controllerutil.RemoveFinalizer(&protectedService, finalizerName)
		return client.Update(ctx, &protectedService)
	}

	if !controllerutil.ContainsFinalizer(&protectedService, finalizerName) {
		controllerutil.AddFinalizer(&protectedService, finalizerName)
		return client.Update(ctx, &protectedService)
	}

	return nil
}
