package operator_cloud_client

import (
	"context"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

const (
	involvedObjectKindField = "involvedObject.kind"
)

type eventKey string
type eventGeneration int64
type intentStatusKey string
type intentStatusDetails struct {
	Generation         int
	UpToDate           bool
	ObservedGeneration int
}

type IntentEventsPeriodicReporter struct {
	cloudClient       CloudClient
	k8sClient         client.Client
	k8sClusterManager ctrl.Manager
	eventCache        *lru.Cache[eventKey, eventGeneration]
	statusCache       *lru.Cache[intentStatusKey, intentStatusDetails]
}

func NewIntentEventsSender(cloudClient CloudClient, k8sClusterManager ctrl.Manager) (*IntentEventsPeriodicReporter, error) {
	eventCache, err := lru.New[eventKey, eventGeneration](1000)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	statusCache, err := lru.New[intentStatusKey, intentStatusDetails](1000)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return &IntentEventsPeriodicReporter{
		cloudClient:       cloudClient,
		k8sClusterManager: k8sClusterManager,
		k8sClient:         k8sClusterManager.GetClient(),
		eventCache:        eventCache,
		statusCache:       statusCache,
	}, nil
}

func (ies *IntentEventsPeriodicReporter) initIndex() error {
	err := ies.k8sClusterManager.GetCache().IndexField(
		context.Background(),
		&v1.Event{},
		involvedObjectKindField,
		func(object client.Object) []string {
			event := object.(*v1.Event)
			return []string{event.InvolvedObject.Kind}
		})

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (ies *IntentEventsPeriodicReporter) Start(ctx context.Context) error {
	err := ies.initIndex()
	if err != nil {
		return errors.Wrap(err)
	}
	go func() {
		defer errorreporter.AutoNotify()
		// Wait for caches to sync
		ies.waitForCacheSync(ctx)

		ies.startReportLoop(ctx)
	}()
	return nil
}

func (ies *IntentEventsPeriodicReporter) startReportLoop(ctx context.Context) {
	// Report events and statuses before starting the ticker
	ies.reportIntentEvents(ctx)
	ies.reportIntentStatuses(ctx)

	eventReportTicker := time.NewTicker(time.Second * time.Duration(viper.GetInt(otterizecloudclient.IntentEventsReportIntervalKey)))
	statusReportTicker := time.NewTicker(time.Second * time.Duration(viper.GetInt(otterizecloudclient.IntentStatusReportIntervalKey)))

	for {
		select {
		case <-ctx.Done():
			return
		case <-eventReportTicker.C:
			ies.reportIntentEvents(ctx)
		case <-statusReportTicker.C:
			ies.reportIntentStatuses(ctx)
		}
	}
}

func (ies *IntentEventsPeriodicReporter) waitForCacheSync(ctx context.Context) {
	for {
		ok := func() bool {
			timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			return ies.k8sClusterManager.GetCache().WaitForCacheSync(timeoutCtx)
		}()
		if !ok {
			logrus.Error("Intents Event Sender - Failed waiting for caches to sync")
			continue
		}
		break
	}
}

func (ies *IntentEventsPeriodicReporter) reportIntentEvents(ctx context.Context) {
	gqlEvents, err := ies.queryIntentEvents(ctx)
	if err != nil {
		logrus.WithError(err).Error("Failed to query intent events")
		return
	}

	if len(gqlEvents) == 0 {
		logrus.WithField("events_in_cache", ies.eventCache.Len()).Debug("No new intent events to report")
		return
	}

	ies.doReportEvents(ctx, gqlEvents)

}

func (ies *IntentEventsPeriodicReporter) doReportEvents(ctx context.Context, gqlEvents []graphqlclient.ClientIntentEventInput) {
	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()
	eventChunks := lo.Chunk(gqlEvents, 100)
	for _, chunk := range eventChunks {
		ies.cloudClient.ReportIntentEvents(timeoutCtx, chunk)
	}
}

func (ies *IntentEventsPeriodicReporter) queryIntentEvents(ctx context.Context) ([]graphqlclient.ClientIntentEventInput, error) {
	events := v1.EventList{}
	gqlEvents := make([]graphqlclient.ClientIntentEventInput, 0)
	err := ies.k8sClient.List(ctx, &events, client.MatchingFields{involvedObjectKindField: "ClientIntents"})
	if err != nil {
		return nil, errors.Wrap(err)
	}
	if len(events.Items) == 0 {
		logrus.Debugf("No events in list")
		return gqlEvents, nil
	}
	for _, event := range events.Items {
		key := eventKey(event.UID)
		if cachedGeneration, ok := ies.eventCache.Get(key); ok && cachedGeneration == eventGeneration(event.Generation) {
			continue

		}
		intent := v2alpha1.ClientIntents{}
		err := ies.k8sClient.Get(ctx, client.ObjectKey{Namespace: event.InvolvedObject.Namespace, Name: event.InvolvedObject.Name}, &intent)
		if err != nil {
			logrus.Errorf("Failed to get intent %s/%s: %v", event.InvolvedObject.Namespace, event.InvolvedObject.Name, err)
			continue
		}
		si := intent.ToServiceIdentity()

		gqlEvents = append(gqlEvents, graphqlclient.ClientIntentEventInput{
			ClientName:         si.Name,
			ClientWorkloadKind: si.Kind,
			Namespace:          si.Namespace,
			Name:               event.Name,
			Labels:             convertMapToKVInput(event.Labels),
			Annotations:        convertMapToKVInput(event.Annotations),
			Count:              int(event.Count),
			ClientIntentName:   event.InvolvedObject.Name,
			FirstTimestamp:     event.FirstTimestamp.Time,
			LastTimestamp:      event.LastTimestamp.Time,
			ReportingComponent: event.ReportingController,
			ReportingInstance:  event.ReportingInstance,
			SourceComponent:    event.Source.Component,
			Type:               event.Type,
			Reason:             event.Reason,
			Message:            event.Message,
		})
		ies.eventCache.Add(key, eventGeneration(event.Generation))
	}
	return gqlEvents, nil
}

func (ies *IntentEventsPeriodicReporter) reportIntentStatuses(ctx context.Context) {
	statuses, err := ies.queryIntentStatuses(ctx)

	if err != nil {
		logrus.WithError(err).Error("Failed to query intent statuses")
		return
	}

	if len(statuses) == 0 {
		logrus.WithField("statuses_in_cache", ies.statusCache.Len()).Debug("No new intent statuses to report")
		return
	}

	ies.doReportStatuses(ctx, statuses)
}

func (ies *IntentEventsPeriodicReporter) doReportStatuses(ctx context.Context, statuses []graphqlclient.ClientIntentStatusInput) {
	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()
	statusChunks := lo.Chunk(statuses, 100)
	for _, chunk := range statusChunks {
		ies.cloudClient.ReportClientIntentStatuses(timeoutCtx, chunk)
	}
}

func (ies *IntentEventsPeriodicReporter) queryIntentStatuses(ctx context.Context) ([]graphqlclient.ClientIntentStatusInput, error) {
	clientIntents := v2alpha1.ClientIntentsList{}
	err := ies.k8sClient.List(ctx, &clientIntents)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	statuses := make([]graphqlclient.ClientIntentStatusInput, 0)
	for _, intent := range clientIntents.Items {
		if cachedDetails, ok := ies.statusCache.Get(intentStatusKey(intent.UID)); ok && cachedDetails.Generation == int(intent.Generation) && cachedDetails.UpToDate == intent.Status.UpToDate && cachedDetails.ObservedGeneration == int(intent.Status.ObservedGeneration) {
			continue
		}
		si := intent.ToServiceIdentity()
		statuses = append(statuses, graphqlclient.ClientIntentStatusInput{
			Namespace:          si.Namespace,
			ClientName:         si.Name,
			ClientWorkloadKind: si.Kind,
			ClientIntentName:   intent.Name,
			Generation:         int(intent.Generation),
			Timestamp:          intent.CreationTimestamp.Time,
			ObservedGeneration: int(intent.Status.ObservedGeneration),
			UpToDate:           intent.Status.UpToDate,
		})
		ies.statusCache.Add(intentStatusKey(intent.UID), intentStatusDetails{
			Generation:         int(intent.Generation),
			UpToDate:           intent.Status.UpToDate,
			ObservedGeneration: int(intent.Status.ObservedGeneration),
		})
	}
	return statuses, nil
}

func convertMapToKVInput(labels map[string]string) []graphqlclient.KeyValueInput {
	var gqlLabels []graphqlclient.KeyValueInput
	for k, v := range labels {
		gqlLabels = append(gqlLabels, graphqlclient.KeyValueInput{
			Key:   k,
			Value: v,
		})
	}
	return gqlLabels
}
