package postgres

import (
	"context"
	"fmt"
	"github.com/amit7itz/goset"
	"github.com/jackc/pgx/v5"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/sqlutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"net/url"
	"strings"
	"sync"
)

const (
	PGCreateUserStatement                      SQLSprintfStatement = "CREATE USER %s WITH PASSWORD %s"
	PGAlterUserPasswordStatement               SQLSprintfStatement = "ALTER USER %s PASSWORD %s"
	PGRevokeAllTableStatement                  SQLSprintfStatement = "REVOKE ALL ON TABLE %s FROM %s"
	PGRevokeAllOnSeqStatement                  SQLSprintfStatement = "REVOKE ALL ON SEQUENCE %s FROM %s"
	PGGrantStatement                           SQLSprintfStatement = "GRANT %s ON %s to %s"
	PGGrantOnAllTablesInSchemaStatement        SQLSprintfStatement = "GRANT %s ON ALL TABLES IN SCHEMA %s to %s"
	PGGrantOnAllSequencesInSchemaStatement     SQLSprintfStatement = "GRANT %s ON ALL SEQUENCES IN SCHEMA %s to %s"
	PGRevokeOnAllTablesInSchemaStatement       SQLSprintfStatement = "REVOKE ALL ON ALL TABLES IN SCHEMA %s FROM %s"
	PGRevokeOnAllSequencesInSchemaStatement    SQLSprintfStatement = "REVOKE ALL ON ALL SEQUENCES IN SCHEMA %s FROM %s"
	PGDropUserStatement                        SQLSprintfStatement = "DROP USER %s"
	PGSelectUserQuery                                              = "SELECT FROM pg_catalog.pg_user WHERE usename = $1"
	PGSelectPrivilegesQuery                                        = "SELECT table_schema, table_name FROM information_schema.table_privileges WHERE grantee = $1"
	PGSSelectTableSequencesPrivilegesQuery                         = "SELECT split_part(column_default, '''', 2) FROM information_schema.columns WHERE column_default LIKE 'nextval%' AND table_schema=$1 AND table_name=$2"
	PGSSelectSchemaNamesQuery                                      = "SELECT schema_name FROM information_schema.schemata WHERE schema_name!='pg_catalog' AND schema_name!='pg_toast' AND schema_name!='information_schema'"
	PGSelectAllDatabasesWithConnectPermissions                     = "SELECT datname FROM pg_catalog.pg_database d WHERE datallowconn AND datistemplate=false AND has_database_privilege($1, d.datname, 'CONNECT');"
	PGDefaultDatabase                                              = "postgres"
)

func databaseConfigInputToSQLTableIdentifier(resource otterizev1alpha3.DatabaseResource) sqlutils.SQLTableIdentifier {
	tableIdentifier := strings.Split(resource.Table, ".")
	if len(tableIdentifier) == 2 {
		return sqlutils.SQLTableIdentifier{TableSchema: tableIdentifier[0], TableName: tableIdentifier[1]}
	}
	return sqlutils.SQLTableIdentifier{TableSchema: "public", TableName: resource.Table}
}

type PostgresDatabaseInfo struct {
	Address     string
	Credentials databaseconfigurator.DatabaseCredentials
}

type PostgresConfigurator struct {
	conn         *pgx.Conn
	databaseInfo PostgresDatabaseInfo
	setConnMutex sync.Mutex
}

func NewPostgresConfigurator(ctx context.Context, databaseInfo PostgresDatabaseInfo) (*PostgresConfigurator, error) {
	p := &PostgresConfigurator{
		databaseInfo: databaseInfo,
		setConnMutex: sync.Mutex{},
	}

	if err := p.setConnection(ctx, PGDefaultDatabase); err != nil {
		pgErr, ok := TranslatePostgresConnectionError(err)
		if ok {
			return nil, errors.Wrap(errors.New(pgErr))
		}
		return nil, errors.Wrap(err)
	}

	return p, nil
}

func (p *PostgresConfigurator) ApplyDatabasePermissionsForUser(ctx context.Context, username string, dbnameToDatabaseResources map[string][]otterizev1alpha3.DatabaseResource) error {
	// apply new intents
	for dbname, dbResources := range dbnameToDatabaseResources {
		if err := p.setConnection(ctx, dbname); err != nil {
			return errors.Wrap(err)
		}

		if err := p.applyDatabasePermissions(ctx, username, dbname, dbResources); err != nil {
			return errors.Wrap(err)
		}

		// revoke excess permissions to tables not specified in the applied intents this database
		allowedTablesForUser, err := p.queryAllowedTablesForUser(ctx, username)
		if err != nil {
			return errors.Wrap(err)
		}

		tablesToRevoke := p.getAllowedTablesDiffForUser(allowedTablesForUser, dbResources)
		if err = p.revokeRemovedTablesPermissions(ctx, tablesToRevoke, username); err != nil {
			return errors.Wrap(err)
		}
	}

	// revoke excess permissions to databases not specified in the applied intents
	databasesToRevoke, err := p.getAllowedDatabasesDiffForUser(ctx, username, lo.Keys(dbnameToDatabaseResources))
	if err != nil {
		return errors.Wrap(err)
	}

	for _, databaseName := range databasesToRevoke {
		if err := p.revokeDatabasePermissions(ctx, username, databaseName); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (p *PostgresConfigurator) getAllowedTablesDiffForUser(allowedTablesForUser []sqlutils.SQLTableIdentifier, intentsDBResources []otterizev1alpha3.DatabaseResource) []sqlutils.SQLTableIdentifier {
	hasWildcardTable := false
	intentsDBSchemaToTables := make(map[string]*goset.Set[string])
	for _, resource := range intentsDBResources {
		if resource.Table == "" {
			hasWildcardTable = true
		} else {
			tableIdentifier := databaseConfigInputToSQLTableIdentifier(resource)
			if _, ok := intentsDBSchemaToTables[tableIdentifier.TableSchema]; !ok {
				intentsDBSchemaToTables[tableIdentifier.TableSchema] = goset.NewSet[string]()
			}
			intentsDBSchemaToTables[tableIdentifier.TableSchema].Add(tableIdentifier.TableName)
		}
	}

	isTableAllowedByIntents := func(table sqlutils.SQLTableIdentifier) bool {
		if hasWildcardTable {
			// the table is allowed by a wildcard intent on the entire DB
			return true
		}
		intentsTables, ok := intentsDBSchemaToTables[table.TableSchema]
		if !ok {
			// the entire schema is not allowed
			return false
		}

		// some allowed tables for this schema, this table is allowed if it is one of them
		return intentsTables.Contains(table.TableName)
	}

	return lo.Filter(allowedTablesForUser, func(table sqlutils.SQLTableIdentifier, _ int) bool {
		return !isTableAllowedByIntents(table)
	})
}

func (p *PostgresConfigurator) RevokeAllDatabasePermissionsForUser(ctx context.Context, username string) error {
	allAllowedDatabases, err := p.queryAllowedDatabases(ctx, username)
	if err != nil {
		return errors.Wrap(err)
	}

	for _, databaseName := range allAllowedDatabases {
		if err := p.revokeDatabasePermissions(ctx, username, databaseName); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (p *PostgresConfigurator) applyDatabasePermissions(ctx context.Context, username string, dbname string, dbResources []otterizev1alpha3.DatabaseResource) error {
	statementsBatch, err := p.sqlBatchFromDBResources(ctx, username, dbResources)
	if err != nil {
		return errors.Wrap(err)
	}
	err = p.sendBatch(ctx, &statementsBatch)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (p *PostgresConfigurator) getAllowedDatabasesDiffForUser(ctx context.Context, username string, intentsDBNames []string) ([]string, error) {
	allAllowedDatabases, err := p.queryAllowedDatabases(ctx, username)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return lo.Without(allAllowedDatabases, intentsDBNames...), nil
}

func (p *PostgresConfigurator) revokeDatabasePermissions(ctx context.Context, username string, dbname string) error {
	err := p.setConnection(ctx, dbname)
	if err != nil && IsInvalidAuthorizationError(err) {
		// Probably an admin database that we're failing to connect to or something along those lines
		// (like 'cloudsqladmin' database in managed Google cloud SQL)
		logrus.WithField("database", dbname).Debug("Skipping invalid authorization error")
		return nil
	}
	if err != nil {
		pgErr, ok := TranslatePostgresConnectionError(err)
		if ok {
			return errors.Wrap(fmt.Errorf(pgErr))
		}
		return errors.Wrap(err)
	}

	batch := &pgx.Batch{}
	if err := p.queueRevokePermissionsByDatabaseNameStatements(ctx, batch, username); err != nil {
		return errors.Wrap(err)
	}
	if err := p.sendBatch(ctx, batch); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (p *PostgresConfigurator) sendBatch(ctx context.Context, statementsBatch *pgx.Batch) error {
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

func (p *PostgresConfigurator) setConnection(ctx context.Context, databaseName string) error {
	connectionString := p.formatConnectionString(databaseName)
	conn, err := pgx.Connect(ctx, connectionString)
	if err != nil {
		return errors.Wrap(err)
	}
	p.setConnMutex.Lock()
	defer p.setConnMutex.Unlock()
	if p.conn != nil {
		if err := p.conn.Close(ctx); err != nil {
			// Intentionally no error returned - clean up error
			logrus.WithField("database", p.databaseInfo.Address).Error("Failed closing connection")
		}
	}
	p.conn = conn
	return nil
}

func (p *PostgresConfigurator) sqlBatchFromDBResources(ctx context.Context, username string, dbResources []otterizev1alpha3.DatabaseResource) (pgx.Batch, error) {
	batch := pgx.Batch{}

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
	postgresTableIdentifier := databaseConfigInputToSQLTableIdentifier(resource)
	rows, err := p.conn.Query(ctx, PGSSelectTableSequencesPrivilegesQuery, postgresTableIdentifier.TableSchema, postgresTableIdentifier.TableName)
	if err != nil {
		return errors.Wrap(err)
	}
	defer rows.Close()

	for rows.Next() {
		var sequenceName string
		if err := rows.Scan(&sequenceName); err != nil {
			return errors.Wrap(err)
		}
		stmt, err := PGGrantStatement.PrepareSanitized([]otterizev1alpha3.DatabaseOperation{otterizev1alpha3.DatabaseOperationAll}, pgx.Identifier{postgresTableIdentifier.TableSchema, sequenceName}, pgx.Identifier{username})
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
	operations := sqlutils.GetGrantOperations(resource.Operations)
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
		logrus.WithField("database", p.databaseInfo.Address).Error("Failed closing connection")
	}
}

func (p *PostgresConfigurator) queryAllowedTablesForUser(ctx context.Context, username string) ([]sqlutils.SQLTableIdentifier, error) {
	allowedTables := make([]sqlutils.SQLTableIdentifier, 0)
	rows, err := p.conn.Query(ctx, PGSelectPrivilegesQuery, username)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	defer rows.Close()
	for rows.Next() {
		var allowedTable sqlutils.SQLTableIdentifier
		if err := rows.Scan(&allowedTable.TableSchema, &allowedTable.TableName); err != nil {
			return nil, errors.Wrap(err)
		}
		logrus.WithField("user", username).WithField("allowedTables", allowedTables).Debug("User has permissions for the following tables", username, allowedTable)
		allowedTables = append(allowedTables, allowedTable)
	}
	return allowedTables, nil
}

func (p *PostgresConfigurator) revokeRemovedTablesPermissions(ctx context.Context, tablesToRevoke []sqlutils.SQLTableIdentifier, username string) error {
	batch := pgx.Batch{}
	for _, table := range tablesToRevoke {
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
		logrus.WithError(err).Error("Failed closing batch results")
	}

	return nil
}

func (p *PostgresConfigurator) queueRevokeAllOnTableAndSequencesStatements(ctx context.Context, batch *pgx.Batch, table sqlutils.SQLTableIdentifier, username string) error {
	rows, err := p.conn.Query(ctx, PGSSelectTableSequencesPrivilegesQuery, table.TableSchema, table.TableName)
	if err != nil {
		return errors.Wrap(err)
	}
	defer rows.Close()

	for rows.Next() {
		var sequenceName string
		if err := rows.Scan(&sequenceName); err != nil {
			return errors.Wrap(err)
		}
		stmt, err := PGRevokeAllOnSeqStatement.PrepareSanitized(pgx.Identifier{table.TableSchema, sequenceName}, pgx.Identifier{username})
		if err != nil {
			return errors.Wrap(err)
		}
		batch.Queue(stmt)
	}

	stmt, err := PGRevokeAllTableStatement.PrepareSanitized(pgx.Identifier{table.TableSchema, table.TableName}, pgx.Identifier{username})
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
		operations := sqlutils.GetGrantOperations(resource.Operations)
		stmt, err = PGGrantOnAllTablesInSchemaStatement.PrepareSanitized(operations, pgx.Identifier{schemaName}, pgx.Identifier{username})
		if err != nil {
			return errors.Wrap(err)
		}
		batch.Queue(stmt)
	}
	return nil
}

func (p *PostgresConfigurator) queueRevokePermissionsByDatabaseNameStatements(ctx context.Context, batch *pgx.Batch, username string) error {
	// Get all schemas in current database
	rows, err := p.conn.Query(ctx, PGSSelectSchemaNamesQuery)
	if err != nil {
		return errors.Wrap(err)
	}
	defer rows.Close()

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

func (p *PostgresConfigurator) formatConnectionString(databaseName string) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s/%s",
		p.databaseInfo.Credentials.Username,
		url.QueryEscape(p.databaseInfo.Credentials.Password),
		p.databaseInfo.Address,
		databaseName)
}

func (p *PostgresConfigurator) ValidateUserExists(ctx context.Context, username string) (bool, error) {
	rows, err := p.conn.Query(ctx, PGSelectUserQuery, username)
	if err != nil {
		return false, errors.Wrap(err)
	}
	defer rows.Close() // "rows" either holds 1 or 0 rows, and we must call Close() before reusing the connection again

	return rows.Next(), nil
}

func (p *PostgresConfigurator) queryAllowedDatabases(ctx context.Context, username string) ([]string, error) {
	allowedDatabases := make([]string, 0)
	rows, err := p.conn.Query(ctx, PGSelectAllDatabasesWithConnectPermissions, username)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	defer rows.Close()
	for rows.Next() {
		var databaseName string
		if err := rows.Scan(&databaseName); err != nil {
			return nil, errors.Wrap(err)
		}
		allowedDatabases = append(allowedDatabases, databaseName)
	}
	return allowedDatabases, nil
}

func (p *PostgresConfigurator) DropUser(ctx context.Context, username string) error {
	batch := pgx.Batch{}
	stmt, err := PGDropUserStatement.PrepareSanitized(pgx.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	batch.Queue(stmt)
	if err := p.sendBatch(ctx, &batch); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (p *PostgresConfigurator) CreateUser(ctx context.Context, username string, password string) error {
	batch := pgx.Batch{}
	stmt, err := PGCreateUserStatement.PrepareSanitized(pgx.Identifier{username}, NonUserInputString(password))
	if err != nil {
		return errors.Wrap(err)
	}
	batch.Queue(stmt)
	if err := p.sendBatch(ctx, &batch); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (p *PostgresConfigurator) AlterUserPassword(ctx context.Context, username string, password string) error {
	batch := pgx.Batch{}
	stmt, err := PGAlterUserPasswordStatement.PrepareSanitized(pgx.Identifier{username}, NonUserInputString(password))
	if err != nil {
		return errors.Wrap(err)
	}
	batch.Queue(stmt)
	if err := p.sendBatch(ctx, &batch); err != nil {
		return errors.Wrap(err)
	}
	return nil
}
