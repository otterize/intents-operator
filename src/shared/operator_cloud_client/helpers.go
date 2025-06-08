package operator_cloud_client

import (
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"time"
)

type AppliedIntentsRequestStatus struct {
	ResourceGeneration graphqlclient.IntentRequestResourceGeneration
	Reason             string
	Status             graphqlclient.AppliedIntentsRequestStatusLabel
	Timestamp          time.Time
}

func translateAppliedIntentsRequestsStatusModel(appliedIntentsRequestsStatuses []graphqlclient.GetAppliedIntentsRequestStatusSyncPendingRequestStatusesAppliedIntentsRequestStatus) []AppliedIntentsRequestStatus {
	result := make([]AppliedIntentsRequestStatus, 0)
	for _, requestStatus := range appliedIntentsRequestsStatuses {
		result = append(result, AppliedIntentsRequestStatus{
			ResourceGeneration: graphqlclient.IntentRequestResourceGeneration{
				ResourceId: requestStatus.ResourceId,
				Generation: requestStatus.Generation,
			},
			Reason: requestStatus.Reason,
			Status: requestStatus.Status,
		})
	}

	return result
}
