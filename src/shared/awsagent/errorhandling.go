package awsagent

import (
	"errors"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"
)

func isNoSuchEntityException(err error) bool {
	if err == nil {
		return false
	}

	var apiError smithy.APIError

	if errors.As(err, &apiError) {
		var noSuchEntityException *types.NoSuchEntityException
		switch {
		case errors.As(apiError, &noSuchEntityException):
			return true
		default:
			return false
		}
	} else {
		return false
	}
}
