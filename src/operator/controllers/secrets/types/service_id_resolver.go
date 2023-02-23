package secretstypes

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceIdResolver interface {
	ResolvePodToServiceIdentity(ctx context.Context, pod *v1.Pod) (serviceidresolver.ServiceIdentity, error)
	GetOwnerObject(ctx context.Context, pod *v1.Pod) (client.Object, error)
}
