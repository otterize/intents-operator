package serviceidresolver

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/podownerresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrPodNotFound = errors.NewSentinelError("pod not found")
)

//+kubebuilder:rbac:groups="apps",resources=deployments;replicasets;daemonsets;statefulsets,verbs=get;list;watch
//+kubebuilder:rbac:groups="batch",resources=jobs;cronjobs,verbs=get;list;watch

type ServiceResolver interface {
	ResolveClientIntentToPod(ctx context.Context, intent v2alpha1.ClientIntents) (corev1.Pod, error)
	ResolvePodToServiceIdentity(ctx context.Context, pod *corev1.Pod) (serviceidentity.ServiceIdentity, error)
	ResolveServiceIdentityToPodSlice(ctx context.Context, identity serviceidentity.ServiceIdentity) ([]corev1.Pod, bool, error)
	ResolveIntentTargetToPod(ctx context.Context, target v2alpha1.Target, intentsObjNamespace string) (corev1.Pod, error)
}

type Resolver struct {
	client client.Client
}

func NewResolver(c client.Client) *Resolver {
	return &Resolver{client: c}
}

func (r *Resolver) ResolvePodToServiceIdentity(ctx context.Context, pod *corev1.Pod) (serviceidentity.ServiceIdentity, error) {
	return podownerresolver.ResolvePodToServiceIdentity(ctx, r.client, pod)
}

func (r *Resolver) ResolveServiceIdentityToPodSlice(ctx context.Context, identity serviceidentity.ServiceIdentity) ([]corev1.Pod, bool, error) {
	labels, ok, err := v2alpha1.ServiceIdentityToLabelsForWorkloadSelection(ctx, r.client, identity)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	if !ok {
		return nil, false, nil
	}

	podList := &corev1.PodList{}
	err = r.client.List(ctx, podList, &client.ListOptions{Namespace: identity.Namespace}, client.MatchingLabels(labels))
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	pods := lo.Filter(podList.Items, func(pod corev1.Pod, _ int) bool { return pod.DeletionTimestamp == nil })

	return pods, len(pods) > 0, nil
}

func (r *Resolver) ResolveClientIntentToPod(ctx context.Context, intent v2alpha1.ClientIntents) (corev1.Pod, error) {
	serviceID := intent.ToServiceIdentity()
	pods, ok, err := r.ResolveServiceIdentityToPodSlice(ctx, serviceID)
	if err != nil {
		return corev1.Pod{}, errors.Wrap(err)
	}
	if !ok {
		return corev1.Pod{}, ErrPodNotFound
	}
	return pods[0], nil
}

func (r *Resolver) ResolveIntentTargetToPod(ctx context.Context, target v2alpha1.Target, intentsObjNamespace string) (corev1.Pod, error) {
	serviceID := target.ToServiceIdentity(intentsObjNamespace)
	pods, ok, err := r.ResolveServiceIdentityToPodSlice(ctx, serviceID)
	if err != nil {
		return corev1.Pod{}, errors.Wrap(err)
	}
	if !ok {
		return corev1.Pod{}, ErrPodNotFound
	}
	return pods[0], nil
}

func (r *Resolver) GetOwnerObject(ctx context.Context, pod *corev1.Pod) (client.Object, error) {
	return podownerresolver.GetOwnerObject(ctx, r.client, pod)
}
