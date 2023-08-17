package pod_reconcilers

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get

type NamespaceWatcher struct {
	client.Client
}

func NewNamespaceWatcher(c client.Client) *NamespaceWatcher {
	return &NamespaceWatcher{c}
}

func (ns *NamespaceWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logrus.Infof("Reconciling due to namespace change: %s", req.Name)
	namespace := &v1.Namespace{}
	err := ns.Get(ctx, req.NamespacedName, namespace)
	if k8serrors.IsNotFound(err) {
		logrus.Infoln("namespace was deleted")
		return ctrl.Result{}, nil
	}

	if err != nil {
		return ctrl.Result{}, err
	}

	if !ns.hasOtterizeLabel(namespace) {
		// Add Otterize namespace label so this namespace is a viable selector in network policies
		updatedNS := namespace.DeepCopy()
		updatedNS.Labels[otterizev1alpha2.OtterizeNamespaceLabelKey] = req.Name
		err := ns.Patch(ctx, updatedNS, client.MergeFrom(namespace))
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (ns *NamespaceWatcher) hasOtterizeLabel(namespace *v1.Namespace) bool {
	_, exists := namespace.Labels[otterizev1alpha2.OtterizeNamespaceLabelKey]
	return exists
}

func (ns *NamespaceWatcher) Register(mgr manager.Manager) error {
	watcher, err := controller.New("namespace-watcher", mgr, controller.Options{
		Reconciler:   ns,
		RecoverPanic: lo.ToPtr(true),
	})
	if err != nil {
		return fmt.Errorf("unable to set up namespace controller: %w", err)
	}

	if err = watcher.Watch(&source.Kind{Type: &v1.Namespace{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("unable to watch Namespaces: %w", err)
	}

	return nil
}
