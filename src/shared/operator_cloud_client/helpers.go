package operator_cloud_client

import (
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"time"
)

type AppliedIntentsRequestStatus struct {
	ID        string
	Reason    string
	Status    graphqlclient.AppliedIntentsRequestStatusLabel
	Timestamp time.Time
}

func translateAppliedIntentsRequestsStatusModel(appliedIntentsRequestsStatuses []graphqlclient.GetAppliedIntentsRequestStatusAppliedIntentsRequestStatus) []AppliedIntentsRequestStatus {
	result := make([]AppliedIntentsRequestStatus, 0)
	for _, requestStatus := range appliedIntentsRequestsStatuses {
		result = append(result, AppliedIntentsRequestStatus{
			ID:     requestStatus.Id,
			Reason: requestStatus.Reason,
			Status: requestStatus.Status,
		})
	}

	return result
}
