package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"net/url"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"sync"
)

const (
	PGRevokeAllTableStatement                  SQLSprintfStatement = "REVOKE ALL ON TABLE %s FROM %s"
	PGRevokeAllOnSeqStatement                  SQLSprintfStatement = "REVOKE ALL ON SEQUENCE %s FROM %s"
	PGGrantStatement                           SQLSprintfStatement = "GRANT %s ON %s to %s"
	PGGrantOnAllTablesInSchemaStatement        SQLSprintfStatement = "GRANT %s ON ALL TABLES IN SCHEMA %s to %s"
	PGGrantOnAllSequencesInSchemaStatement     SQLSprintfStatement = "GRANT %s ON ALL SEQUENCES IN SCHEMA %s to %s"
	PGRevokeOnAllTablesInSchemaStatement       SQLSprintfStatement = "REVOKE ALL ON ALL TABLES IN SCHEMA %s FROM %s"
	PGRevokeOnAllSequencesInSchemaStatement    SQLSprintfStatement = "REVOKE ALL ON ALL SEQUENCES IN SCHEMA %s FROM %s"
	PGSelectUserQuery                                              = "SELECT FROM pg_catalog.pg_user where usename = $1"
	PGSelectPrivilegesQuery                                        = "SELECT table_schema, table_name FROM information_schema.table_privileges where grantee = $1"
	PGSSelectTableSequencesPrivilegesQuery                         = "SELECT split_part(column_default, '''', 2) FROM information_schema.columns WHERE column_default LIKE 'nextval%' and table_schema=$1 and table_name=$2"
	PGSSelectSchemaNamesQuery                                      = "SELECT schema_name From information_schema.schemata where schema_name!='pg_catalog' and schema_name!='pg_toast' and schema_name!='information_schema'"
	PGSelectAllDatabasesWithConnectPermissions                     = "SELECT datname from pg_catalog.pg_database d where datallowconn and datistemplate=false and has_database_privilege(d.datname, 'CONNECT');"
	PGDefaultDatabase                                              = "postgres"
)

type SQLSprintfStatement string
type NonUserInputString string

func tableNameToIdentifier(tableName string) pgx.Identifier {
	return strings.Split(tableName, ".")
}

type PostgresTableIdentifier struct {
	tableSchema string
	tableName   string
}

func (p PostgresTableIdentifier) ToPGXIdentifier() pgx.Identifier {
	return pgx.Identifier{p.tableSchema, p.tableName}
}

func (s SQLSprintfStatement) PrepareSanitized(a ...any) (string, error) {
	sanitizedItems := make([]any, len(a))
	for i, formatInput := range a {
		if dbOperations, ok := formatInput.([]otterizev1alpha3.DatabaseOperation); ok {
			asStrings := lo.Map(dbOperations, func(op otterizev1alpha3.DatabaseOperation, _ int) string { return string(op) })
			sanitizedItems[i] = strings.Join(asStrings, ",")
			continue
		}

		// String that was marked explicitly as non-user doesn't have to be sanitized
		if inputStr, ok := formatInput.(NonUserInputString); ok {
			sanitizedItems[i] = fmt.Sprintf("'%s'", inputStr)
			continue
		}

		// Postgres identifiers like table names, users, column names, etc..
		if ident, ok := formatInput.(pgx.Identifier); ok {
			sanitizedItems[i] = ident.Sanitize()
			continue
		}

		return "", errors.Errorf("received sanitize input '%v' with type '%T', which was not string or DatabaseOperation", formatInput, formatInput)
	}
	return fmt.Sprintf(string(s), sanitizedItems...), nil
}

type PostgresConfigurator struct {
	conn         *pgx.Conn
	databaseInfo otterizev1alpha3.PostgreSQLServerConfigSpec
	setConnMutex sync.Mutex
	client       client.Client
}

func NewPostgresConfigurator(pgServerConfSpec otterizev1alpha3.PostgreSQLServerConfigSpec, client client.Client) *PostgresConfigurator {
	return &PostgresConfigurator{
		databaseInfo: pgServerConfSpec,
		setConnMutex: sync.Mutex{},
		client:       client,
	}
}

func (p *PostgresConfigurator) ConfigureDBFromIntents(
	ctx context.Context,
	workloadName string,
	namespace string,
	intents []otterizev1alpha3.Intent,
	permissionChange otterizev1alpha3.DBPermissionChange) error {

	if err := p.SetConnection(ctx, PGDefaultDatabase); err != nil {
		return errors.Wrap(err)
	}

	dbnameToDatabaseResources, err := p.ExtractDBNameToDatabaseResourcesFromIntents(ctx, intents)
	if err != nil {
		return errors.Wrap(err)
	}
	clusterID, err := clusterutils.GetClusterUID(ctx)
	if err != nil {
		return err
	}
	username := clusterutils.BuildHashedUsername(workloadName, namespace, clusterID)
	pgUsername := clusterutils.KubernetesToPostgresName(username)
	exists, err := p.ValidateUserExists(ctx, pgUsername)
	if err != nil {
		return errors.Wrap(err)
	}

	if !exists {
		logrus.WithField("username", pgUsername).Info(
			"Waiting for Postgres user to be created before configuring permissions")
		return errors.Wrap(fmt.Errorf(
			"user for workload %s.%s doesn't exist in DB yet, cannot configure permissions", workloadName, namespace))
	}

	for dbname, dbResources := range dbnameToDatabaseResources {
		if err := p.SetConnection(ctx, dbname); err != nil {
			return errors.Wrap(err)
		}

		if permissionChange != otterizev1alpha3.DBPermissionChangeDelete {
			// Need to check whether tables were deleted from intents, and revoke permissions for them
			allowedTablesDiff, err := p.getAllowedTablesDiffForUser(ctx, pgUsername, dbResources)
			if err != nil {
				return errors.Wrap(err)
			}

			if err = p.revokeRemovedTablesPermissions(ctx, allowedTablesDiff, pgUsername); err != nil {
				return errors.Wrap(err)
			}
		}

		statementsBatch, err := p.SQLBatchFromDBResources(ctx, pgUsername, dbResources, permissionChange)
		if err != nil {
			return errors.Wrap(err)
		}
		err = p.SendBatch(ctx, &statementsBatch)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	return nil

}

func (p *PostgresConfigurator) SendBatch(ctx context.Context, statementsBatch *pgx.Batch) error {
	batchResults := p.conn.SendBatch(ctx, statementsBatch)

	for i := 0; i < statementsBatch.Len(); i++ {
		if _, err := batchResults.Exec(); err != nil {
			return TranslatePostgresCommandsError(err)
		}
	}
	if err := batchResults.Close(); err != nil {
		// Intentionally no error returned - clean up error
		logrus.WithError(err).Errorf("Failed closing batch results")
	}
	return nil
}

func (p *PostgresConfigurator) SetConnection(ctx context.Context, databaseName string) error {
	connectionString := p.FormatConnectionString(databaseName)
	conn, err := pgx.Connect(ctx, connectionString)
	if err != nil {
		pgErr, ok := TranslatePostgresConnectionError(err)
		if ok {
			return errors.Wrap(fmt.Errorf(pgErr))
		}
		return errors.Wrap(err)
	}
	p.setConnMutex.Lock()
	defer p.setConnMutex.Unlock()
	if p.conn == nil {
		p.conn = conn
		return nil
	}
	if err := p.conn.Close(ctx); err != nil {
		// Intentionally no error returned - clean up error
		logrus.Errorf("Failed closing connection to: %s", p.databaseInfo.Address)
	}
	p.conn = conn
	return nil
}

func (p *PostgresConfigurator) ExtractDBNameToDatabaseResourcesFromIntents(ctx context.Context, intents []otterizev1alpha3.Intent) (map[string][]otterizev1alpha3.DatabaseResource, error) {
	scopeToDatabaseResources := make(map[string][]otterizev1alpha3.DatabaseResource)
	for _, intent := range intents {
		for _, dbResource := range intent.DatabaseResources {
			if _, ok := scopeToDatabaseResources[dbResource.DatabaseName]; !ok {
				scopeToDatabaseResources[dbResource.DatabaseName] = []otterizev1alpha3.DatabaseResource{dbResource}
				continue
			}
			// TODO: Smart merge instead of just adding
			resources := scopeToDatabaseResources[dbResource.DatabaseName]
			scopeToDatabaseResources[dbResource.DatabaseName] = append(resources, dbResource)
		}
	}
	rows, err := p.conn.Query(ctx, PGSelectAllDatabasesWithConnectPermissions)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	defer rows.Close()
	for rows.Next() {
		var databaseName string
		if err := rows.Scan(&databaseName); err != nil {
			return nil, errors.Wrap(err)
		}
		if _, exists := scopeToDatabaseResources[databaseName]; exists {
			continue
		}
		scopeToDatabaseResources[databaseName] = []otterizev1alpha3.DatabaseResource{}
	}
	return scopeToDatabaseResources, nil
}

func (p *PostgresConfigurator) SQLBatchFromDBResources(ctx context.Context, username string, dbResources []otterizev1alpha3.DatabaseResource, change otterizev1alpha3.DBPermissionChange) (pgx.Batch, error) {

	batch := pgx.Batch{}

	if change == otterizev1alpha3.DBPermissionChangeDelete {
		// Intent was deleted, revoke all client permissions from mentioned tables
		for _, resource := range dbResources {
			if resource.Table == "" {
				err := p.QueueRevokePermissionsByDatabaseNameStatements(ctx, &batch, username)
				if err != nil {
					return pgx.Batch{}, errors.Wrap(err)
				}
				continue
			}
			err := p.queueRevokeAllOnTableAndSequencesStatements(ctx, &batch, databaseConfigInputToPostgresTableIdentifier(resource), username)
			if err != nil {
				return pgx.Batch{}, errors.Wrap(err)
			}
		}
		return batch, nil
	}

	// Intent was created or updated, so we revoke current permissions and grant new ones
	for _, resource := range dbResources {
		if resource.Table == "" {
			err := p.queueAddPermissionsByDatabaseNameStatements(ctx, &batch, resource, username)
			if err != nil {
				return pgx.Batch{}, errors.Wrap(err)
			}
			continue
		}
		err := p.queueAddPermissionsToTableStatements(ctx, &batch, resource, username)
		if err != nil {
			return pgx.Batch{}, errors.Wrap(err)
		}
	}
	return batch, nil
}

func (p *PostgresConfigurator) queueAddPermissionsToTableStatements(ctx context.Context, batch *pgx.Batch, resource otterizev1alpha3.DatabaseResource, username string) error {
	postgresTableIdentifier := databaseConfigInputToPostgresTableIdentifier(resource)
	rows, err := p.conn.Query(ctx, PGSSelectTableSequencesPrivilegesQuery, postgresTableIdentifier.tableSchema, postgresTableIdentifier.tableName)
	if err != nil {
		return errors.Wrap(err)
	}
	defer rows.Close()

	for rows.Next() {
		var sequenceName string
		if err := rows.Scan(&sequenceName); err != nil {
			return errors.Wrap(err)
		}
		stmt, err := PGGrantStatement.PrepareSanitized([]otterizev1alpha3.DatabaseOperation{otterizev1alpha3.DatabaseOperationAll}, pgx.Identifier{postgresTableIdentifier.tableSchema, sequenceName}, pgx.Identifier{username})
		if err != nil {
			return errors.Wrap(err)
		}
		batch.Queue(stmt)
	}
	// We always include the "revoke all" statement to make sure deleted permissions are removed
	stmt, err := PGRevokeAllTableStatement.PrepareSanitized(tableNameToIdentifier(resource.Table), pgx.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	batch.Queue(stmt)
	operations := p.getGrantOperations(resource.Operations)
	stmt, err = PGGrantStatement.PrepareSanitized(operations, tableNameToIdentifier(resource.Table), pgx.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	batch.Queue(stmt)
	return nil
}

// CloseConnection closes the connection to the database and logs an error
// We cannot use defer on the close connection since it runs in a for loop, and can potentially keep a lot of
// connections open until closing them, which Postgres doesn't like that much
func (p *PostgresConfigurator) CloseConnection(ctx context.Context) {
	if p.conn == nil {
		return
	}
	if err := p.conn.Close(ctx); err != nil {
		// Intentionally no error returned - clean up error
		logrus.Errorf("Failed closing connection to: %s", p.databaseInfo.Address)
	}
}

func (p *PostgresConfigurator) getGrantOperations(operations []otterizev1alpha3.DatabaseOperation) []otterizev1alpha3.DatabaseOperation {

	if len(operations) == 0 || slices.Contains(operations, otterizev1alpha3.DatabaseOperationAll) {
		// Omit everything else in case it's included to avoid Postgres errors, include just 'ALL'
		return []otterizev1alpha3.DatabaseOperation{otterizev1alpha3.DatabaseOperationAll}
	}

	return operations

}

// getAllowedTablesDiffForUser gets the diff between all current tables with permissions for 'username',
// and tables specified in the intent. If tables with current permissions do not exist in the intent all permissions
// from them should be removed, as they were deleted from the ClientIntents resource.
func (p *PostgresConfigurator) getAllowedTablesDiffForUser(ctx context.Context, username string, dbResources []otterizev1alpha3.DatabaseResource) ([]PostgresTableIdentifier, error) {
	intentTables := lo.Map(dbResources, func(resource otterizev1alpha3.DatabaseResource, _ int) PostgresTableIdentifier {
		return databaseConfigInputToPostgresTableIdentifier(resource)
	})
	allowedTablesDiff := make([]PostgresTableIdentifier, 0)
	rows, err := p.conn.Query(ctx, PGSelectPrivilegesQuery, username)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	defer rows.Close()
	for rows.Next() {
		var allowedTable PostgresTableIdentifier
		if err := rows.Scan(&allowedTable.tableSchema, &allowedTable.tableName); err != nil {
			return nil, errors.Wrap(err)
		}
		logrus.Debugf("User %s has permissions for allowedTable: %s", username, allowedTable)
		if !slices.Contains(intentTables, allowedTable) {
			allowedTablesDiff = append(allowedTablesDiff, allowedTable)
		}
	}

	return allowedTablesDiff, nil
}

func databaseConfigInputToPostgresTableIdentifier(resource otterizev1alpha3.DatabaseResource) PostgresTableIdentifier {
	tableIdentifier := strings.Split(resource.Table, ".")
	if len(tableIdentifier) == 2 {
		return PostgresTableIdentifier{tableSchema: tableIdentifier[0], tableName: tableIdentifier[1]}
	}
	return PostgresTableIdentifier{tableSchema: "public", tableName: resource.Table}
}

func (p *PostgresConfigurator) revokeRemovedTablesPermissions(ctx context.Context, allowedTablesDiff []PostgresTableIdentifier, username string) error {
	batch := pgx.Batch{}
	for _, table := range allowedTablesDiff {
		err := p.queueRevokeAllOnTableAndSequencesStatements(ctx, &batch, table, username)
		if err != nil {
			return errors.Wrap(err)
		}
	}
	batchResults := p.conn.SendBatch(ctx, &batch)
	for i := 0; i < batch.Len(); i++ {
		if _, err := batchResults.Exec(); err != nil {
			return TranslatePostgresCommandsError(err)
		}
	}
	if err := batchResults.Close(); err != nil {
		logrus.WithError(err).Errorf("Failed closing batch results")
	}

	return nil
}

func (p *PostgresConfigurator) queueRevokeAllOnTableAndSequencesStatements(ctx context.Context, batch *pgx.Batch, table PostgresTableIdentifier, username string) error {
	rows, err := p.conn.Query(ctx, PGSSelectTableSequencesPrivilegesQuery, table.tableSchema, table.tableName)
	if err != nil {
		return errors.Wrap(err)
	}
	defer rows.Close()

	for rows.Next() {
		var sequenceName string
		if err := rows.Scan(&sequenceName); err != nil {
			return errors.Wrap(err)
		}
		stmt, err := PGRevokeAllOnSeqStatement.PrepareSanitized(pgx.Identifier{table.tableSchema, sequenceName}, pgx.Identifier{username})
		if err != nil {
			return errors.Wrap(err)
		}
		batch.Queue(stmt)
	}

	stmt, err := PGRevokeAllTableStatement.PrepareSanitized(table.ToPGXIdentifier(), pgx.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	batch.Queue(stmt)
	return nil
}

func (p *PostgresConfigurator) queueAddPermissionsByDatabaseNameStatements(ctx context.Context, batch *pgx.Batch, resource otterizev1alpha3.DatabaseResource, username string) error {
	// Get all schemas in current database
	rows, err := p.conn.Query(ctx, PGSSelectSchemaNamesQuery)
	if err != nil {
		return errors.Wrap(err)
	}
	defer rows.Close()

	// Grant privileges on all tables/sequences in every schema
	for rows.Next() {
		var schemaName string
		if err := rows.Scan(&schemaName); err != nil {
			return errors.Wrap(err)
		}
		stmt, err := PGGrantOnAllSequencesInSchemaStatement.PrepareSanitized([]otterizev1alpha3.DatabaseOperation{otterizev1alpha3.DatabaseOperationAll}, pgx.Identifier{schemaName}, pgx.Identifier{username})
		if err != nil {
			return errors.Wrap(err)
		}
		batch.Queue(stmt)
		operations := p.getGrantOperations(resource.Operations)
		stmt, err = PGGrantOnAllTablesInSchemaStatement.PrepareSanitized(operations, pgx.Identifier{schemaName}, pgx.Identifier{username})
		if err != nil {
			return errors.Wrap(err)
		}
		batch.Queue(stmt)
	}
	return nil
}

func (p *PostgresConfigurator) QueueRevokePermissionsByDatabaseNameStatements(ctx context.Context, batch *pgx.Batch, username string) error {
	// Get all schemas in current database
	rows, err := p.conn.Query(ctx, PGSSelectSchemaNamesQuery)
	if err != nil {
		return errors.Wrap(err)
	}
	defer rows.Close()

	// Grant privileges on all tables/sequences in every schema
	for rows.Next() {
		var schemaName string
		if err := rows.Scan(&schemaName); err != nil {
			return errors.Wrap(err)
		}
		stmt, err := PGRevokeOnAllSequencesInSchemaStatement.PrepareSanitized(pgx.Identifier{schemaName}, pgx.Identifier{username})
		if err != nil {
			return errors.Wrap(err)
		}
		batch.Queue(stmt)
		stmt, err = PGRevokeOnAllTablesInSchemaStatement.PrepareSanitized(pgx.Identifier{schemaName}, pgx.Identifier{username})
		if err != nil {
			return errors.Wrap(err)
		}
		batch.Queue(stmt)
	}
	return nil
}

func (p *PostgresConfigurator) FormatConnectionString(databaseName string) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s/%s",
		p.databaseInfo.Credentials.Username,
		url.QueryEscape(p.databaseInfo.Credentials.Password),
		p.databaseInfo.Address,
		databaseName)
}

func (p *PostgresConfigurator) ValidateUserExists(ctx context.Context, user string) (bool, error) {
	row, err := p.conn.Query(ctx, PGSelectUserQuery, user)
	if err != nil {
		return false, errors.Wrap(err)
	}
	defer row.Close() // "row" either holds 1 or 0 rows, and we must call Close() before reusing the connection again

	return row.Next(), nil
}
