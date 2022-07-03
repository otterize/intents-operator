package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	spire_client "github.com/otterize/spifferize/src/spire-client"
	entryv1 "github.com/spiffe/spire-api-sdk/proto/spire/api/server/entry/v1"
	"github.com/spiffe/spire-api-sdk/proto/spire/api/types"
	"google.golang.org/grpc/codes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Log         logr.Logger
	Scheme      *runtime.Scheme
	SpireClient spire_client.ServerClient
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch

const (
	serviceNameLabel = "otterize/service-name"
)

func (r *PodReconciler) registerPodSpireEntry(ctx context.Context, pod *corev1.Pod) error {
	serviceName := pod.Labels[serviceNameLabel]
	log := r.Log.WithValues("pod", pod.Name, "namespace", pod.Namespace, "serviceName", serviceName)

	entry := types.Entry{
		SpiffeId: &types.SPIFFEID{
			TrustDomain: r.SpireClient.ClientConf().TrustDomain().String(),
			Path:        fmt.Sprintf("/otterize/env/%s/service/%s", pod.Namespace, serviceName),
		},
		ParentId: &types.SPIFFEID{
			TrustDomain: r.SpireClient.ClientConf().ClientSpiffeID().TrustDomain().String(),
			Path:        r.SpireClient.ClientConf().ClientSpiffeID().Path(),
		},
		Selectors: []*types.Selector{
			{Type: "k8s", Value: fmt.Sprintf("ns:%s", pod.Namespace)},
			{Type: "k8s", Value: fmt.Sprintf("pod-label:%s=%s", serviceNameLabel, serviceName)},
		},
	}

	log.Info("Creating SPIRE server entry")
	entryClient := r.SpireClient.NewEntryClient()
	batchCreateEntryRequest := entryv1.BatchCreateEntryRequest{Entries: []*types.Entry{&entry}}

	resp, err := entryClient.BatchCreateEntry(ctx, &batchCreateEntryRequest)
	if err != nil {
		return err
	}

	if len(resp.Results) != 1 {
		return fmt.Errorf("unexpected number of results returned from SPIRE server: %d", len(resp.Results))
	}

	result := resp.Results[0]
	switch result.Status.Code {
	case int32(codes.OK):
		log.Info("SPIRE server entry created", "id", result.Entry.Id)
	case int32(codes.AlreadyExists):
		log.Info("SPIRE server entry already exists", "id", result.Entry.Id)
	default:
		return fmt.Errorf("entry failed to create with status %s", result.Status)
	}

	return nil
}

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("pod", req.NamespacedName)

	// Fetch the Pod from the Kubernetes API.
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			// we'll ignore not-found errors, since they can't be fixed by an immediate
			// requeue (we'll need to wait for a new notification), and we can get them
			// on deleted requests.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch Pod")
		return ctrl.Result{}, err
	}

	// Add spire-server entry for pod
	if pod.Labels == nil || pod.Labels[serviceNameLabel] == "" {
		log.Info("no update required - service name label not found")
		return ctrl.Result{}, nil
	}

	if err := r.registerPodSpireEntry(ctx, &pod); err != nil {
		log.Error(err, "failed registering SPIRE entry for pod")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
