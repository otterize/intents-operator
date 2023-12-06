package prometheus

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	intentsApplied = promauto.NewCounter(prometheus.CounterOpts{
		Name: "clientintents_applied",
		Help: "The total number of ClientIntents applied",
	})
	netpolCreated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "network_policies_created",
		Help: "The total number of network policies created",
	})
	netpolDeleted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "network_policies_deleted",
		Help: "The total number of network policies deleted",
	})
	podsLabeledForAccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pods_labeled_for_network_policy",
		Help: "The total number of pods labeled to participate in a network policy",
	})
	podsUnlabeledForAccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pods_unlabeled_for_network_policy",
		Help: "The total number of pods unlabeled so they no longer participate in a network policy",
	})
	protectedServiceApplied = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "protected_services_applied",
		Help: "The total number of ProtectedService resources applied",
	})
)

func IncrementIntentsApplied(count int) {
	intentsApplied.Add(float64(count))
}

func IncrementNetpolCreated(count int) {
	netpolCreated.Add(float64(count))
}

func IncrementNetpolDeleted(count int) {
	netpolDeleted.Add(float64(count))
}

func IncrementPodsLabeledForNetworkPolicies(count int) {
	podsLabeledForAccess.Add(float64(count))
}
func IncrementPodsUnlabeledForNetworkPolicies(count int) {
	podsUnlabeledForAccess.Add(float64(count))
}

func SetProtectedServicesApplied(count int) {
	protectedServiceApplied.Set(float64(count))
}
