package operator_cloud_client

import "github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"

type IntentsApprovalResult struct {
	ID     string
	Reason string
	Status graphqlclient.AccessRequestStatus
}

func translateLatestApprovalHistoryModel(getApprovalHistoryResult []graphqlclient.GetIntentsApprovalHistoryGetIntentsApprovalHistoryAccessRequest) []IntentsApprovalResult {
	result := make([]IntentsApprovalResult, 0)
	for _, item := range getApprovalHistoryResult {
		result = append(result, IntentsApprovalResult{
			ID:     item.Id,
			Reason: item.Reason,
			Status: item.Status,
		})
	}

	return result
}
