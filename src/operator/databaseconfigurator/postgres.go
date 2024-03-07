package databaseconfigurator

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/otterize/cloud/src/backend-service/pkg/graph/model"
	"github.com/otterize/cloud/src/backend-service/pkg/lib/apis"
	"github.com/otterize/cloud/src/backend-service/pkg/lib/errors"
	"github.com/otterize/cloud/src/backend-service/pkg/lib/errors/usererrors"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/types"
	"net"
	"net/url"
	"regexp"
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
	PGSelectAllDatabasesWithConnectPermissions                     = "SELECT datname from pg_catalog.pg_database d where datallowconn and datistemplate=false and has_database_privilege(d.datname, 'CONNECT');" 	= "postgres"
)

const pgUsernameMaxLength = 63

var serviceIDRegexPattern = regexp.MustCompile(`^(svc_[a-zA-Z\d]+)_.+$`)

type SQLSprintfStatement string
type NonUserInputString string

func tableNameToIdentifier(tableName string) pgx.Identifier {
	return strings.Split(tableName, ".")
}

type PostgresTableIdentifier struct {
	tableSchema string
	tableName   string
}

func (p PostgresTableIdentifier) toPGXIdentifier() pgx.Identifier {
	return pgx.Identifier{p.tableSchema, p.tableName}
}

func (s SQLSprintfStatement) PrepareSanitized(a ...any) (string, error) {
	sanitizedItems := make([]any, len(a))
	for i, formatInput := range a {
		// We don't need to sanitize []model.DatabaseOperation because they were already validated by the API (It's an enum)
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

func BuildPostgresUsername(client types.NamespacedName) string {
	username := fmt.Sprintf("%s_%s", KubernetesToPostgresName(client.Namespace), KubernetesToPostgresName(client.Name))
	if len(username) > pgUsernameMaxLength {
		username = username[:pgUsernameMaxLength]
	}
	return username
}

func ServiceIDFromPostgresUsername(username string) apis.ServiceID {
	match := serviceIDRegexPattern.FindStringSubmatch(username)
	if len(match) == 0 {
		return ""
	}

	return apis.ServiceID(match[1])
}

func TranslatePostgresConnectionError(err error) (string, bool) {
	if opErr := &(net.OpError{}); errors.As(err, &opErr) || errors.Is(err, context.DeadlineExceeded) {
		return "Can't reach the server", true
	}

	if connErr := &(pgconn.ConnectError{}); errors.As(err, &connErr) {
		return "Can't reach the server", true
	}

	if dnsErr := &(net.DNSError{}); errors.As(err, &dnsErr) {
		return "Can't resolve hostname", true
	}

	if pgErr := &(pgconn.PgError{}); errors.As(err, &pgErr) {
		// See: https://www.postgresql.org/docs/current/errcodes-appendix.html
		if pgErr.Code == "28P01" || pgErr.Code == "28000" {
			return "Invalid credentials", true
		}
		if pgErr.Code == "3D000" {
			return fmt.Sprintf("Database doesn't exist: %s", pgErr.Message), true
		}
	}

	return "", false
}

func IsInvalidAuthorizationError(err error) bool {
	if pgErr := &(pgconn.PgError{}); errors.As(err, &pgErr) {
		if pgErr.Code == "28000" {
			return true
		}
	}
	return false
}

func translatePostgresCommandsError(err error) error {
	if pgErr := &(pgconn.PgError{}); errors.As(err, &pgErr) {
		// See: https://www.postgresql.org/docs/current/errcodes-appendix.html
		if pgErr.Code == "42P01" || pgErr.Code == "3F000" {
			return usererrors.AppliedIntentsError("Bad schema/table name: %s", pgErr.Message)
		}
	}
	return errors.Wrap(err)
}

func (p *PostgresConfigurator) ConfigureDBFromIntents(
	ctx context.Context,
	workloadNamespacedName types.NamespacedName,
	intents []otterizev1alpha3.Intent,
	permissionChange otterizev1alpha3.DBPermissionChange) error {

	connectionString := p.formatConnectionString()
	conn, err := pgx.Connect(ctx, connectionString)
	if err != nil {
		pgErr, ok := TranslatePostgresConnectionError(err)
		if ok {
			return usererrors.AppliedIntentsError(pgErr)
		}
		return errors.Wrap(err)
	}
	p.setConnection(ctx, conn)
	dbnameToDatabaseResources, err := p.extractDBNameToDatabaseResourcesFromIntents(ctx, intents)
	if err != nil {
		return errors.Wrap(err)
	}
	username := BuildPostgresUsername(workloadNamespacedName)

	exists, err := p.validateUserExists(ctx, username)
	if err != nil {
		return errors.Wrap(err)
	}
	if !exists {
		logrus.WithField("username", username).Info(
			"Waiting for Postgres user to be created before configuring permissions")
		return ct

	}

	for dbname, dbResources := range dbnameToDatabaseResources {
		connectionString = p.formatConnectionString()
		conn, err = pgx.Connect(ctx, connectionString)
		// If we got invalid authorization error at this point it means we are blocked by the hba_conf file.
		// It may occur for cloud provider databases such as "cloudsqladmin".
		// TODO: Remove it when we have a better solution for knowing what were the last applied intents.
		if err != nil && IsInvalidAuthorizationError(err) {
			logrus.Infof("Invalid authorization error to: %s", dbname)
			continue
		}
		if err != nil {
			pgErr, ok := TranslatePostgresConnectionError(err)
			if ok {
				return usererrors.AppliedIntentsError(pgErr)
			}
			return errors.Wrap(err)
		}
		p.setConnection(ctx, conn)
		if permissionChange != otterizev1alpha3.DBPermissionChangeDelete {
			// Need to check whether tables were deleted from intents, and revoke permissions for them
			allowedTablesDiff, err := p.getAllowedTablesDiffForUser(ctx, username, dbResources)
			if err != nil {
				return errors.Wrap(err)
			}

			if err = p.revokeRemovedTablesPermissions(ctx, allowedTablesDiff, username); err != nil {
				return errors.Wrap(err)
			}
		}

		statementsBatch, err := p.sqlBatchFromDBResources(ctx, username, dbResources, permissionChange)
		if err != nil {
			return errors.Wrap(err)
		}
		batchResults := p.conn.SendBatch(ctx, &statementsBatch)

		for i := 0; i < statementsBatch.Len(); i++ {
			if _, err := batchResults.Exec(); err != nil {
				return translatePostgresCommandsError(err)
			}
		}
		if err = batchResults.Close(); err != nil {
			// Intentionally no error returned - clean up error
			logrus.WithError(err).Errorf("Failed closing batch results")
		}

	}
	return nil

}

func (p *PostgresConfigurator) setConnection(ctx context.Context, conn *pgx.Conn) {
	p.setConnMutex.Lock()
	defer p.setConnMutex.Unlock()
	if p.conn == nil {
		p.conn = conn
		return
	}
	if err := p.conn.Close(ctx); err != nil {
		// Intentionally no error returned - clean up error
		logrus.Errorf("Failed closing connection to: %s", p.databaseInfo.Address)
	}
	p.conn = conn
}

func (p *PostgresConfigurator) extractDBNameToDatabaseResourcesFromIntents(ctx context.Context, intents []otterizev1alpha3.Intent) (map[string][]otterizev1alpha3.DatabaseResource, error) {
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

func (p *PostgresConfigurator) sqlBatchFromDBResources(
	ctx context.Context,
	username string,
	dbResources []model.DatabaseConfigInput,
	change model.DBPermissionChange) (pgx.Batch, error) {

	batch := pgx.Batch{}

	if change == model.DBPermissionChangeDelete {
		// Intent was deleted, revoke all client permissions from mentioned tables
		for _, resource := range dbResources {
			if lo.FromPtr(resource.Table) == "" {
				err := p.queueRevokePermissionsByDatabaseNameStatements(ctx, &batch, username)
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
		if lo.FromPtr(resource.Table) == "" {
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

func (p *PostgresConfigurator) queueAddPermissionsToTableStatements(ctx context.Context, batch *pgx.Batch, resource model.DatabaseConfigInput, username string) error {
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
		stmt, err := PGGrantStatement.PrepareSanitized([]model.DatabaseOperation{model.DatabaseOperationAll}, pgx.Identifier{postgresTableIdentifier.tableSchema, sequenceName}, pgx.Identifier{username})
		if err != nil {
			return errors.Wrap(err)
		}
		batch.Queue(stmt)
	}
	// We always include the "revoke all" statement to make sure deleted permissions are removed
	stmt, err := PGRevokeAllTableStatement.PrepareSanitized(tableNameToIdentifier(lo.FromPtr(resource.Table)), pgx.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	batch.Queue(stmt)
	operations := p.getGrantOperations(resource.Operations)
	stmt, err = PGGrantStatement.PrepareSanitized(operations, tableNameToIdentifier(lo.FromPtr(resource.Table)), pgx.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	batch.Queue(stmt)
	return nil
}

func (p *PostgresConfigurator) validateUserExists(ctx context.Context, user string) (bool, error) {
	row, err := p.conn.Query(ctx, PGSelectUserQuery, user)
	if err != nil {
		return false, errors.Wrap(err)
	}
	defer row.Close() // "row" either holds 1 or 0 rows, and we must call Close() before reusing the connection again

	return row.Next(), nil
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

func (p *PostgresConfigurator) getGrantOperations(
	operations []model.DatabaseOperation) []model.DatabaseOperation {

	if len(operations) == 0 || slices.Contains(operations, model.DatabaseOperationAll) {
		// Omit everything else in case it's included to avoid Postgres errors, include just 'ALL'
		return []model.DatabaseOperation{model.DatabaseOperationAll}
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
			return translatePostgresCommandsError(err)
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

	stmt, err := PGRevokeAllTableStatement.PrepareSanitized(table.toPGXIdentifier(), pgx.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	batch.Queue(stmt)
	return nil
}

func (p *PostgresConfigurator) queueAddPermissionsByDatabaseNameStatements(ctx context.Context, batch *pgx.Batch, resource model.DatabaseConfigInput, username string) error {
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
		stmt, err := PGGrantOnAllSequencesInSchemaStatement.PrepareSanitized([]model.DatabaseOperation{model.DatabaseOperationAll}, pgx.Identifier{schemaName}, pgx.Identifier{username})
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

func (p *PostgresConfigurator) queueRevokePermissionsByDatabaseNameStatements(ctx context.Context, batch *pgx.Batch, username string) error {
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

func (p *PostgresConfigurator) formatConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s/%s",
		p.databaseInfo.Credentials.Username,
		url.QueryEscape(p.databaseInfo.Credentials.Password),
		p.databaseInfo.Address,
		p.databaseInfo.DatabaseName)
}

// KubernetesToPostgresName translates a name with Kubernetes conventions to Postgres conventions
func KubernetesToPostgresName(kubernetesName string) string {
	// '.' are replaced with dunders '__'
	// '-' are replaced with single underscores '_'
	return strings.ReplaceAll(strings.ReplaceAll(kubernetesName, ".", "__"), "-", "_")
}

// Commented out to avoid linter hell
//func postgresToKubernetesName(pgName string) string {
//	return strings.ReplaceAll(strings.ReplaceAll(pgName, "__", "."), "_", "-")
//}
