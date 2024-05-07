package mysql

import (
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"strings"
)

type SQLSprintfStatement string
type NonUserInputString string
type UserDefinedIdentifier string

func (i UserDefinedIdentifier) Sanitize() string {
	s := string(i)
	s = strings.ReplaceAll(s, string([]byte{0}), "")
	s = strings.ReplaceAll(s, "'", "")
	return s
}

func (s SQLSprintfStatement) PrepareSanitized(a ...any) (string, error) {
	sanitizedItems := make([]any, len(a))
	for i, formatInput := range a {
		sanitizedItem, err := sanitizeFormatInput(formatInput)
		if err != nil {
			return "", errors.Wrap(err)
		}
		sanitizedItems[i] = sanitizedItem
	}
	return fmt.Sprintf(string(s), sanitizedItems...), nil
}

func sanitizeFormatInput(formatInput any) (string, error) {
	if dbOperations, ok := formatInput.([]otterizev1alpha3.DatabaseOperation); ok {
		asStrings := lo.Map(dbOperations, func(op otterizev1alpha3.DatabaseOperation, _ int) string { return string(op) })
		return strings.Join(asStrings, ","), nil
	}

	// String that was marked explicitly as non-user doesn't have to be sanitized
	if inputStr, ok := formatInput.(NonUserInputString); ok {
		return fmt.Sprintf("'%s'", inputStr), nil
	}

	// Postgres identifiers like table names, users, column names, etc..
	if ident, ok := formatInput.(UserDefinedIdentifier); ok {
		return ident.Sanitize(), nil
	}

	return "", errors.Errorf("received sanitize input '%v' with type '%T', which was not string or DatabaseOperation", formatInput, formatInput)
}
