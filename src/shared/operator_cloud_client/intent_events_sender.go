package operator_cloud_client

import (
	"context"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	k8errors "k8s.io/apimachinery/pkg/api/errors"
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

type IntentEvent struct {
	Event  v1.Event
	Intent v2alpha1.ClientIntents
}

type IntentEventsPeriodicReporter struct {
	cloudClient       CloudClient
	k8sClient         client.Client
	k8sClusterManager ctrl.Manager
	eventCache        *expirable.LRU[eventKey, eventGeneration]
	statusCache       *expirable.LRU[intentStatusKey, intentStatusDetails]
}

func NewIntentEventsSender(cloudClient CloudClient, k8sClusterManager ctrl.Manager) (*IntentEventsPeriodicReporter, error) {
	eventCache := expirable.NewLRU[eventKey, eventGeneration](1000, nil, time.Hour)
	statusCache := expirable.NewLRU[intentStatusKey, intentStatusDetails](1000, nil, time.Hour)

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
			timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
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
	intentEvents, err := ies.queryIntentEvents(ctx)
	if err != nil {
		logrus.WithError(err).Error("Failed to query intent events")
		return
	}

	if len(intentEvents) == 0 {
		logrus.WithField("events_in_cache", ies.eventCache.Len()).Debug("No new intent events to report")
		return
	}

	logrus.WithField("events", len(intentEvents)).Info("Reporting intent events")

	err = ies.doReportEvents(ctx, intentEvents)
	if err != nil {
		logrus.WithError(err).Error("Failed to report intent events")
		return
	}

	ies.cacheReportedEvents(intentEvents)
}

func (ies *IntentEventsPeriodicReporter) doReportEvents(ctx context.Context, intentEvents []IntentEvent) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	gqlEvents := lo.Map(intentEvents, func(intentEvent IntentEvent, _ int) graphqlclient.ClientIntentEventInput {
		return eventToGQLEvent(intentEvent.Intent, intentEvent.Event)
	})

	eventChunks := lo.Chunk(gqlEvents, 100)
	for _, chunk := range eventChunks {
		err := ies.cloudClient.ReportIntentEvents(timeoutCtx, chunk)
		if err != nil {
			return errors.Wrap(err)
		}
	}
	return nil
}

func (ies *IntentEventsPeriodicReporter) queryIntentEvents(ctx context.Context) ([]IntentEvent, error) {
	events := v1.EventList{}
	err := ies.k8sClient.List(ctx, &events, client.MatchingFields{involvedObjectKindField: "ClientIntents"})
	if err != nil {
		return nil, errors.Wrap(err)
	}

	filteredEvents := lo.Filter(events.Items, func(event v1.Event, _ int) bool {
		key := eventKey(event.UID)
		cachedGeneration, ok := ies.eventCache.Get(key)
		return !ok || cachedGeneration != eventGeneration(event.Generation)
	})

	intentEvents := lo.FilterMap(filteredEvents, func(event v1.Event, _ int) (IntentEvent, bool) {
		intent := v2alpha1.ClientIntents{}
		err := ies.k8sClient.Get(ctx, client.ObjectKey{Namespace: event.InvolvedObject.Namespace, Name: event.InvolvedObject.Name}, &intent)
		if err != nil {
			if !k8errors.IsNotFound(err) {
				logrus.Errorf("Failed to get intent %s/%s: %v", event.InvolvedObject.Namespace, event.InvolvedObject.Name, err)
			}
			return IntentEvent{}, false
		}

		return IntentEvent{Event: event, Intent: intent}, true
	})

	return intentEvents, nil
}

func (ies *IntentEventsPeriodicReporter) cacheReportedEvents(intentEvents []IntentEvent) {
	for _, intentEvent := range intentEvents {
		ies.eventCache.Add(eventKey(intentEvent.Event.UID), eventGeneration(intentEvent.Event.Generation))
	}
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

	logrus.WithField("statuses", len(statuses)).Info("Reporting intent statuses")

	err = ies.doReportStatuses(ctx, statuses)
	if err != nil {
		logrus.WithError(err).Error("Failed to report intent statuses")
		return
	}

	ies.cacheReportedIntentStatuses(statuses)
}

func (ies *IntentEventsPeriodicReporter) doReportStatuses(ctx context.Context, intents []v2alpha1.ClientIntents) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	gqlStatuses := lo.Map(intents, func(intent v2alpha1.ClientIntents, _ int) graphqlclient.ClientIntentStatusInput {
		return statusToGQLStatus(intent)
	})

	statusChunks := lo.Chunk(gqlStatuses, 100)
	for _, chunk := range statusChunks {
		err := ies.cloudClient.ReportClientIntentStatuses(timeoutCtx, chunk)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (ies *IntentEventsPeriodicReporter) queryIntentStatuses(ctx context.Context) ([]v2alpha1.ClientIntents, error) {
	clientIntents := v2alpha1.ClientIntentsList{}
	err := ies.k8sClient.List(ctx, &clientIntents)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	filteredIntents := lo.Filter(clientIntents.Items, func(intent v2alpha1.ClientIntents, _ int) bool {
		cachedDetails, ok := ies.statusCache.Get(intentStatusKey(intent.UID))
		return !ok ||
			cachedDetails.Generation != int(intent.Generation) ||
			cachedDetails.UpToDate != intent.Status.UpToDate ||
			cachedDetails.ObservedGeneration != int(intent.Status.ObservedGeneration)
	})

	return filteredIntents, nil
}

func (ies *IntentEventsPeriodicReporter) cacheReportedIntentStatuses(intents []v2alpha1.ClientIntents) {
	for _, intent := range intents {
		ies.statusCache.Add(
			intentStatusKey(intent.UID),
			intentStatusDetails{
				Generation:         int(intent.Generation),
				UpToDate:           intent.Status.UpToDate,
				ObservedGeneration: int(intent.Status.ObservedGeneration),
			},
		)
	}
}

func statusToGQLStatus(intent v2alpha1.ClientIntents) graphqlclient.ClientIntentStatusInput {
	si := intent.ToServiceIdentity()
	gqlStatus := graphqlclient.ClientIntentStatusInput{
		Namespace:          si.Namespace,
		ClientName:         si.Name,
		ClientWorkloadKind: si.Kind,
		ClientIntentName:   intent.Name,
		Generation:         int(intent.Generation),
		Timestamp:          intent.CreationTimestamp.Time,
		ObservedGeneration: int(intent.Status.ObservedGeneration),
		UpToDate:           intent.Status.UpToDate,
	}
	return gqlStatus
}

func eventToGQLEvent(intent v2alpha1.ClientIntents, event v1.Event) graphqlclient.ClientIntentEventInput {
	si := intent.ToServiceIdentity()
	gqlEvent := graphqlclient.ClientIntentEventInput{
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
	}
	return gqlEvent
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
