package access_annotation

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Intent struct {
	Client        serviceidentity.ServiceIdentity
	Server        serviceidentity.ServiceIdentity
	EventRecorder *injectablerecorder.ObjectEventRecorder
}

type IntentsByService map[serviceidentity.ServiceIdentity][]Intent

type AnnotationIntents struct {
	IntentsByServer IntentsByService
	IntentsByClient IntentsByService
}

type ServiceIdResolver interface {
	ResolvePodToServiceIdentity(ctx context.Context, pod *v1.Pod) (serviceidentity.ServiceIdentity, error)
}

func GetIntentsInCluster(
	ctx context.Context,
	k8sClient client.Client,
	serviceIdResolver ServiceIdResolver,
	recorder *injectablerecorder.InjectableRecorder,
) (AnnotationIntents, error) {
	annotationIntents := AnnotationIntents{
		IntentsByServer: make(IntentsByService),
		IntentsByClient: make(IntentsByService),
	}

	podList, err := getAllAnnotationIntentsServers(ctx, k8sClient)
	if err != nil {
		return AnnotationIntents{}, errors.Wrap(err)
	}

	for _, pod := range podList.Items {
		if pod.GetDeletionTimestamp() != nil {
			continue
		}

		clients, ok, err := ParseAccessAnnotations(&pod)
		if err != nil {
			return AnnotationIntents{}, errors.Wrap(err)
		}
		if !ok {
			continue
		}
		serverIdentity, err := serviceIdResolver.ResolvePodToServiceIdentity(ctx, &pod)
		if err != nil {
			return AnnotationIntents{}, errors.Wrap(err)
		}
		for _, clientIdentity := range clients {
			intent := Intent{
				Client:        clientIdentity,
				Server:        serverIdentity,
				EventRecorder: injectablerecorder.NewObjectEventRecorder(recorder, lo.ToPtr(pod)),
			}
			annotationIntents.addIntent(intent)
		}
	}
	return annotationIntents, nil
}

func getAllAnnotationIntentsServers(ctx context.Context, k8sClient client.Client) (v1.PodList, error) {
	var podList v1.PodList
	err := k8sClient.List(
		ctx,
		&podList,
		client.MatchingFields{otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationIndexField: otterizev1alpha3.OtterizeServerHasAnyCalledByAnnotationValue},
	)
	if err != nil {
		return v1.PodList{}, errors.Wrap(err)
	}
	return podList, nil
}

func (a *AnnotationIntents) addIntent(intent Intent) {
	serverIdentity := intent.Server
	existing, ok := a.IntentsByServer[serverIdentity]
	if !ok {
		existing = make([]Intent, 0)
	}
	existing = append(existing, intent)
	a.IntentsByServer[serverIdentity] = existing

	clientIdentity := intent.Client
	existing, ok = a.IntentsByClient[clientIdentity]
	if !ok {
		existing = make([]Intent, 0)
	}
	existing = append(existing, intent)
	a.IntentsByClient[clientIdentity] = existing
}
