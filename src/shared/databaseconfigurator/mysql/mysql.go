package mysql

import (
	"context"
	"database/sql"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/sqlutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	"net/url"
	"sync"
)

const (
	SQLCreateUserStatement sqlutils.SQLSprintfStatement = "CREATE USER %s IDENTIFIED BY %s"
	SQLSelectUserQuery     sqlutils.SQLSprintfStatement = "SELECT 1 FROM mysql.user WHERE user = $1"
	SQLDropUserQuery       sqlutils.SQLSprintfStatement = "DROP USER %s"

	SQLGrantPermissionsOnDatabaseStatement    sqlutils.SQLSprintfStatement = "GRANT %s ON %s.* TO %s"
	SQLGrantAllPermissionsOnDatabaseStatement sqlutils.SQLSprintfStatement = "GRANT ALL PRIVILEGES ON %s.* TO %s"
	SQLGrantPermissionsOnTableStatement       sqlutils.SQLSprintfStatement = "GRANT %s ON %s.%s TO %s"
	SQLGrantAllPermissionsOnTableStatement    sqlutils.SQLSprintfStatement = "GRANT ALL PRIVILEGES ON %s.%s TO %s"

	SQLRevokeAllPermissionsForUserStatement    sqlutils.SQLSprintfStatement = "REVOKE ALL PRIVILEGES ON *.* FROM %s"
	SQLRevokeAllPermissionsOnDatabaseStatement sqlutils.SQLSprintfStatement = "REVOKE ALL PRIVILEGES ON %s.* FROM %s"
	SQLRevokeAllPermissionsOnTableStatement    sqlutils.SQLSprintfStatement = "REVOKE ALL PRIVILEGES ON %s FROM %s"

	SQLSelectDatabasesWithUserPrivilegesQuery sqlutils.SQLSprintfStatement = `select i.table_schema, i.table_name from information_schema.tables i
left join mysql.user u on u.user = $1 and u.select_priv = 'Y'
left join mysql.db d on d.user = $1 and d.db = i.table_schema and d.select_priv = 'Y'
left join mysql.tables_priv t on t.user = $1 and t.db = i.table_schema and t.table_name = i.table_name and LENGTH(t.table_priv) > 0
where coalesce(u.user, d.user, t.user) is not null`
	DefaultDatabase = "mysql"
)

// TODO: this should be defined in otterizev1alpha3
type MySQLServerConfigSpec struct {
	Address     string                               `json:"address"`
	Credentials otterizev1alpha3.DatabaseCredentials `json:"credentials"`
}

type MySQLConfigurator struct {
	databaseInfo MySQLServerConfigSpec

	db         *sql.DB
	setDBMutex sync.Mutex
}

func NewMySQLConfigurator(ctx context.Context, conf MySQLServerConfigSpec) (*MySQLConfigurator, error) {
	m := &MySQLConfigurator{
		databaseInfo: conf,
		setDBMutex:   sync.Mutex{},
	}

	if err := m.setConnection(DefaultDatabase); err != nil {
		return nil, errors.Wrap(err)
	}

	// TODO: ping DB?
	return m, nil
}

func (m *MySQLConfigurator) formatConnectionString(databaseName string) string {
	return fmt.Sprintf(
		// multiStatements=true to allow multiple statements in a single query
		"%s:%s@%s/%s?multiStatements=true",
		m.databaseInfo.Credentials.Username,
		url.QueryEscape(m.databaseInfo.Credentials.Password),
		m.databaseInfo.Address,
		databaseName)
}

func (m *MySQLConfigurator) setConnection(databaseName string) error {
	connectionString := m.formatConnectionString(databaseName)
	db, err := sql.Open("mysql", connectionString)
	if err != nil {
		return errors.Wrap(err)
	}

	m.setDBMutex.Lock()
	defer m.setDBMutex.Unlock()
	if m.db != nil {
		if err := m.db.Close(); err != nil {
			// Intentionally no error returned - clean up error
			logrus.Errorf("Failed closing connection to: %s", m.databaseInfo.Address)
		}
	}
	m.db = db

	return nil
}

func (m *MySQLConfigurator) CreateUser(ctx context.Context, username string, password string) error {
	stmt, err := SQLCreateUserStatement.PrepareSanitized(sqlutils.Identifier{username}, sqlutils.NonUserInputString(password))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) ValidateUserExists(ctx context.Context, username string) (bool, error) {
	stmt, err := SQLSelectUserQuery.PrepareSanitized()
	if err != nil {
		return false, errors.Wrap(err)
	}
	rows, err := m.db.QueryContext(ctx, stmt, username)
	if err != nil {
		return false, errors.Wrap(err)
	}
	defer rows.Close()

	return rows.Next(), nil
}

func (m *MySQLConfigurator) DropUser(ctx context.Context, username string) error {
	stmt, err := SQLDropUserQuery.PrepareSanitized(sqlutils.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}

	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) ApplyDatabasePermissionsForUser(ctx context.Context, username string, dbnameToDatabaseResources map[string][]otterizev1alpha3.DatabaseResource) error {
	allowedTablesForUser, err := m.queryAllowedTablesForUser(ctx, username)
	if err != nil {
		return errors.Wrap(err)
	}

	// apply new intents
	for dbname, dbResources := range dbnameToDatabaseResources {
		for _, dbResource := range dbResources {
			if err := m.applyDatabaseResourcePermissions(ctx, username, dbname, dbResource); err != nil {
				return errors.Wrap(err)
			}
		}
	}

	// revoke excess permissions to tables not specified in the applied intents for database that were otherwise specified
	allowedTablesDiff := sqlutils.DiffIntentsTables(allowedTablesForUser, dbnameToDatabaseResources)
	for _, table := range allowedTablesDiff {
		if err := m.revokeTablePermissions(ctx, username, table); err != nil {
			return errors.Wrap(err)
		}
	}

	// revoke excess permissions to databases not specified in the applied intents
	databasesToRevoke := sqlutils.DiffIntentsDBs(allowedTablesForUser, dbnameToDatabaseResources)
	for _, databaseName := range databasesToRevoke {
		if err := m.revokeDatabasePermissions(ctx, username, databaseName); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (m *MySQLConfigurator) queryAllowedTablesForUser(ctx context.Context, username string) ([]sqlutils.SQLTableIdentifier, error) {
	stmt, err := SQLSelectDatabasesWithUserPrivilegesQuery.PrepareSanitized()
	if err != nil {
		return nil, errors.Wrap(err)
	}

	rows, err := m.db.QueryContext(ctx, stmt, username)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	defer rows.Close()

	allowedTables := make([]sqlutils.SQLTableIdentifier, 0)
	for rows.Next() {
		var allowedTable sqlutils.SQLTableIdentifier
		if err := rows.Scan(&allowedTable.TableSchema, &allowedTable.TableName); err != nil {
			return nil, errors.Wrap(err)
		}
		allowedTables = append(allowedTables, allowedTable)
	}

	return allowedTables, nil
}

func (m *MySQLConfigurator) applyDatabaseResourcePermissions(ctx context.Context, username string, dbname string, dbResource otterizev1alpha3.DatabaseResource) error {
	operations := sqlutils.GetGrantOperations(dbResource.Operations)
	containsAllOperation := sqlutils.ContainsAllOperations(operations)
	if dbResource.Table == "" {
		// Apply permissions on database level
		if containsAllOperation {
			// Apply all permissions on database level
			return m.grantAllPermissionsOnDatabase(ctx, username, dbname)
		} else {
			// Apply permissions on database level
			return m.grantPermissionsOnDatabase(ctx, username, dbname, operations)
		}
	} else {
		// Apply permissions on table level
		if containsAllOperation {
			// Apply all permissions on table level
			return m.grantAllPermissionsOnTable(ctx, username, dbname, dbResource.Table)
		} else {
			// Apply permissions on table level
			return m.grantPermissionsOnTable(ctx, username, dbname, dbResource.Table, operations)
		}
	}
}

func (m *MySQLConfigurator) grantAllPermissionsOnDatabase(ctx context.Context, username string, dbname string) error {
	stmt, err := SQLGrantAllPermissionsOnDatabaseStatement.PrepareSanitized(sqlutils.Identifier{dbname}, sqlutils.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) grantPermissionsOnDatabase(ctx context.Context, username string, dbname string, operations []otterizev1alpha3.DatabaseOperation) error {
	// TODO: diff and revoke excess permissions
	stmt, err := SQLGrantPermissionsOnDatabaseStatement.PrepareSanitized(operations, sqlutils.Identifier{dbname}, sqlutils.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) grantAllPermissionsOnTable(ctx context.Context, username string, dbname string, tableName string) error {
	stmt, err := SQLGrantAllPermissionsOnTableStatement.PrepareSanitized(sqlutils.Identifier{dbname}, sqlutils.Identifier{tableName}, sqlutils.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) grantPermissionsOnTable(ctx context.Context, username string, dbname string, tableName string, operations []otterizev1alpha3.DatabaseOperation) error {
	// TODO: diff and revoke excess permissions
	stmt, err := SQLGrantPermissionsOnTableStatement.PrepareSanitized(operations, sqlutils.Identifier{dbname}, sqlutils.Identifier{tableName}, sqlutils.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) revokeTablePermissions(ctx context.Context, username string, table sqlutils.SQLTableIdentifier) error {
	stmt, err := SQLRevokeAllPermissionsOnTableStatement.PrepareSanitized(table.ToSQLIdentifier(), sqlutils.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) revokeDatabasePermissions(ctx context.Context, username string, dbname string) error {
	stmt, err := SQLRevokeAllPermissionsOnDatabaseStatement.PrepareSanitized(sqlutils.Identifier{dbname}, sqlutils.Identifier{username})
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) RevokeAllDatabasePermissionsForUser(ctx context.Context, pgUsername string) error {
	stmt, err := SQLRevokeAllPermissionsForUserStatement.PrepareSanitized(sqlutils.Identifier{pgUsername})
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}
