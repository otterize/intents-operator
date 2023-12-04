package graphqlclient

import (
	"errors"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func GetGraphQLUserError(err error) (UserErrorType, string, bool) {
	errList := &(gqlerror.List{})
	if !errors.As(err, errList) {
		return "", "", false
	}
	gqlErr := &gqlerror.Error{}
	if !errList.As(&gqlErr) {
		return "", "", false
	}
	errType, ok := gqlErr.Extensions["errorType"].(string)
	if !ok || !UserErrorType(errType).IsValid() {
		return "", "", false
	}
	return UserErrorType(errType), gqlErr.Message, true
}

func (e UserErrorType) IsValid() bool {
	switch e {
	case UserErrorTypeUnauthenticated, UserErrorTypeNotFound, UserErrorTypeInternalServerError, UserErrorTypeBadRequest, UserErrorTypeForbidden, UserErrorTypeConflict, UserErrorTypeBadUserInput, UserErrorTypeAppliedIntentsError:
		return true
	}
	return false
}

func (e UserErrorType) String() string {
	return string(e)
}
