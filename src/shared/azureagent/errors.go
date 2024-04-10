package azureagent

import azureerrors "github.com/Azure/azure-sdk-for-go-extensions/pkg/errors"

const (
	ParentResourceNotFound = "ParentResourceNotFound"
)

// IsParentResourceNotFoundErr is used to determine if we are failing to find a resource because its parent was not found
func IsParentResourceNotFoundErr(err error) bool {
	azErr := azureerrors.IsResponseError(err)
	return azErr != nil && azErr.ErrorCode == ParentResourceNotFound
}
