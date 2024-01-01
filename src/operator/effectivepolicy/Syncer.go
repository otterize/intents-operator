package effectivepolicy

import (
	"context"
	goerrors "errors"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EffectivePoliciesApplier interface {
	ApplyEffectivePolicies(ctx context.Context, eps []ServiceEffectivePolicy) (int, error)
	InjectRecorder(recorder record.EventRecorder)
}

type Syncer struct {
	client.Client
	Scheme   *runtime.Scheme
	appliers []EffectivePoliciesApplier
	injectablerecorder.InjectableRecorder
}

func NewSyncer(k8sClient client.Client, scheme *runtime.Scheme, appliers ...EffectivePoliciesApplier) *Syncer {
	return &Syncer{
		Client:   k8sClient,
		Scheme:   scheme,
		appliers: appliers,
	}
}

func (s *Syncer) AddApplier(applier EffectivePoliciesApplier) {
	s.appliers = append(s.appliers, applier)
}

func (s *Syncer) InjectRecorder(recorder record.EventRecorder) {
	for _, applier := range s.appliers {
		applier.InjectRecorder(recorder)
	}
}

func (s *Syncer) Sync(ctx context.Context) error {
	eps, err := GetAllServiceEffectivePolicies(ctx, s.Client, &s.InjectableRecorder)
	if err != nil {
		return errors.Wrap(err)
	}

	errorList := make([]error, 0)
	for _, applier := range s.appliers {
		_, err := applier.ApplyEffectivePolicies(ctx, eps)
		if err != nil {
			errorList = append(errorList, errors.Wrap(err))
		}
	}
	return goerrors.Join(errorList...)
}
