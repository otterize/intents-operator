package access_annotation

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/nilable"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AdditionalAccess struct {
	// Client is not required - if not set, all access is allowed.
	Client        nilable.Nilable[serviceidentity.ServiceIdentity]
	Server        serviceidentity.ServiceIdentity
	EventRecorder *injectablerecorder.ObjectEventRecorder
}

type AllAdditionalAccess struct {
	IntentsByServer map[serviceidentity.ServiceIdentity][]AdditionalAccess
	IntentsByClient map[serviceidentity.ServiceIdentity][]AdditionalAccess
}

type ServiceIdResolver interface {
	ResolvePodToServiceIdentity(ctx context.Context, pod *v1.Pod) (serviceidentity.ServiceIdentity, error)
}

func GetAllAdditionalAccessFromCluster(
	ctx context.Context,
	k8sClient client.Client,
	serviceIdResolver ServiceIdResolver,
	recorder *injectablerecorder.InjectableRecorder,
) (AllAdditionalAccess, error) {
	annotationIntents := AllAdditionalAccess{
		IntentsByServer: make(map[serviceidentity.ServiceIdentity][]AdditionalAccess),
		IntentsByClient: make(map[serviceidentity.ServiceIdentity][]AdditionalAccess),
	}

	podList, err := getAllAnnotationIntentsServers(ctx, k8sClient)
	if err != nil {
		return AllAdditionalAccess{}, errors.Wrap(err)
	}

	for _, pod := range podList.Items {
		if pod.GetDeletionTimestamp() != nil {
			continue
		}

		clients, ok, err := ParseAdditionalAccess(&pod)
		if err != nil {
			return AllAdditionalAccess{}, errors.Wrap(err)
		}
		if !ok {
			continue
		}
		serverIdentity, err := serviceIdResolver.ResolvePodToServiceIdentity(ctx, &pod)
		if err != nil {
			return AllAdditionalAccess{}, errors.Wrap(err)
		}
		for _, clientIdentity := range clients {
			intent := AdditionalAccess{
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

func (a *AllAdditionalAccess) addIntent(intent AdditionalAccess) {
	serverIdentity := intent.Server
	existing, ok := a.IntentsByServer[serverIdentity]
	if !ok {
		existing = make([]AdditionalAccess, 0)
	}
	existing = append(existing, intent)
	a.IntentsByServer[serverIdentity] = existing

	clientIdentity := intent.Client
	existing, ok = a.IntentsByClient[clientIdentity]
	if !ok {
		existing = make([]AdditionalAccess, 0)
	}
	existing = append(existing, intent)
	a.IntentsByClient[clientIdentity] = existing
}
