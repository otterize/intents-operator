package mysql

import (
	"fmt"
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
	if privileges, ok := formatInput.([]PrivilegeType); ok {
		asStrings := lo.Map(privileges, func(op PrivilegeType, _ int) string { return op.String() })
		return strings.Join(asStrings, ","), nil
	}

	// String that was marked explicitly as non-user doesn't have to be sanitized
	if inputStr, ok := formatInput.(NonUserInputString); ok {
		return fmt.Sprintf("'%s'", inputStr), nil
	}

	// MySQL identifiers like table names, users, column names, etc..
	if ident, ok := formatInput.(UserDefinedIdentifier); ok {
		return ident.Sanitize(), nil
	}

	return "", errors.Errorf("received sanitize input '%v' with type '%T', which was not string or DatabaseOperation", formatInput, formatInput)
}
