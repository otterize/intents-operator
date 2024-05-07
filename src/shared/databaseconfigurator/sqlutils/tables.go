package sqlutils

import (
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/samber/lo"
	"strings"
)

func TableNameToIdentifier(tableName string) Identifier {
	return strings.Split(tableName, ".")
}

type SQLTableIdentifier struct {
	TableSchema string
	TableName   string
}

func (i SQLTableIdentifier) ToSQLIdentifier() Identifier {
	return Identifier{i.TableSchema, i.TableName}
}

func DatabaseConfigInputToSQLTableIdentifier(resource otterizev1alpha3.DatabaseResource) SQLTableIdentifier {
	tableIdentifier := strings.Split(resource.Table, ".")
	if len(tableIdentifier) == 2 {
		return SQLTableIdentifier{TableSchema: tableIdentifier[0], TableName: tableIdentifier[1]}
	}
	return SQLTableIdentifier{TableSchema: "public", TableName: resource.Table}
}

func DiffIntentsTables(sqlTables []SQLTableIdentifier, dbnameToDatabaseResources map[string][]otterizev1alpha3.DatabaseResource) []SQLTableIdentifier {
	intentTables := lo.Map(lo.Flatten(lo.Values(dbnameToDatabaseResources)), func(resource otterizev1alpha3.DatabaseResource, _ int) SQLTableIdentifier {
		return DatabaseConfigInputToSQLTableIdentifier(resource)
	})
	return lo.Without(sqlTables, intentTables...)
}

func DiffIntentsDBs(sqlTables []SQLTableIdentifier, dbnameToDatabaseResources map[string][]otterizev1alpha3.DatabaseResource) []string {
	allAllowedDatabases := lo.Uniq(lo.Map(sqlTables, func(t SQLTableIdentifier, _ int) string {
		return t.TableSchema
	}))

	return lo.Without(allAllowedDatabases, lo.Keys(dbnameToDatabaseResources)...)
}
