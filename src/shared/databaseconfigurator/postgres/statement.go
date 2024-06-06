package postgres

import (
	"fmt"
	"github.com/jackc/pgx/v5"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"strings"
)

type SQLSprintfStatement string
type NonUserInputString string

func tableNameToIdentifier(tableName string) pgx.Identifier {
	return strings.Split(tableName, ".")
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
	if dbOperations, ok := formatInput.([]otterizev2alpha1.DatabaseOperation); ok {
		asStrings := lo.Map(dbOperations, func(op otterizev2alpha1.DatabaseOperation, _ int) string { return string(op) })
		return strings.Join(asStrings, ","), nil
	}

	// String that was marked explicitly as non-user doesn't have to be sanitized
	if inputStr, ok := formatInput.(NonUserInputString); ok {
		return fmt.Sprintf("'%s'", inputStr), nil
	}

	// Postgres identifiers like table names, users, column names, etc..
	if ident, ok := formatInput.(pgx.Identifier); ok {
		return ident.Sanitize(), nil
	}

	return "", errors.Errorf("received sanitize input '%v' with type '%T', which was not string or DatabaseOperation", formatInput, formatInput)
}
