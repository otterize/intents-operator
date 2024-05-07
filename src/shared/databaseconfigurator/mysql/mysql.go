package mysql

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/sqlutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	sqldblogger "github.com/simukti/sqldb-logger"
	"github.com/simukti/sqldb-logger/logadapter/logrusadapter"
	"github.com/sirupsen/logrus"
	"net/url"
	"sync"
)

const (
	SQLCreateUserStatement SQLSprintfStatement = "CREATE USER %s IDENTIFIED BY %s"
	SQLDropUserQuery       SQLSprintfStatement = "DROP USER %s"

	SQLGrantPermissionsOnDatabaseStatement    SQLSprintfStatement = "GRANT %s ON %s.* TO '%s'"
	SQLGrantAllPermissionsOnDatabaseStatement SQLSprintfStatement = "GRANT ALL ON %s.* TO '%s'"
	SQLGrantPermissionsOnTableStatement       SQLSprintfStatement = "GRANT %s ON %s.%s TO '%s'"
	SQLGrantAllPermissionsOnTableStatement    SQLSprintfStatement = "GRANT ALL ON %s.%s TO '%s'"

	SQLRevokeAllPermissionsForUserStatement    SQLSprintfStatement = "REVOKE ALL ON *.* FROM '%s'"
	SQLRevokeAllPermissionsOnDatabaseStatement SQLSprintfStatement = "REVOKE ALL ON %s.* FROM '%s'"
	SQLRevokeAllPermissionsOnTableStatement    SQLSprintfStatement = "REVOKE ALL ON %s FROM '%s'"

	SQLSelectUserQuery = "SELECT 1 FROM mysql.user WHERE user = ?"
	// TODO: use template as to avoid passing the username multiple times
	SQLSelectDatabasesWithUserPrivilegesQuery = `select i.table_schema, i.table_name from information_schema.tables i
left join mysql.user u on u.user = ? and u.select_priv = 'Y'
left join mysql.db d on d.user = ? and d.db = i.table_schema and d.select_priv = 'Y'
left join mysql.tables_priv t on t.user = ? and t.db = i.table_schema and t.table_name = i.table_name and LENGTH(t.table_priv) > 0
where coalesce(u.user, d.user, t.user) is not null`
	DefaultDatabase = "mysql"
)

type MySQLConfigurator struct {
	databaseInfo otterizev1alpha3.PostgreSQLServerConfigSpec // TODO: Use MySQLServerConfigSpec

	db         *sql.DB
	setDBMutex sync.Mutex
}

func NewMySQLConfigurator(ctx context.Context, conf otterizev1alpha3.PostgreSQLServerConfigSpec) (*MySQLConfigurator, error) {
	m := &MySQLConfigurator{
		databaseInfo: conf,
		setDBMutex:   sync.Mutex{},
	}

	if err := m.setConnection(DefaultDatabase); err != nil {
		return nil, errors.Wrap(err)
	}

	if err := m.db.PingContext(ctx); err != nil {
		defer m.Close()
		return nil, errors.Wrap(err)
	}

	return m, nil
}

func (m *MySQLConfigurator) formatConnectionString(databaseName string) string {
	return fmt.Sprintf(
		// multiStatements=true to allow multiple statements in a single query
		"%s:%s@tcp(%s)/%s",
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

	db = sqldblogger.OpenDriver(connectionString, db.Driver(), logrusadapter.New(logrus.StandardLogger()))

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
	stmt, err := SQLCreateUserStatement.PrepareSanitized(UserDefinedIdentifier(username), NonUserInputString(password))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) ValidateUserExists(ctx context.Context, username string) (bool, error) {
	rows, err := m.db.QueryContext(ctx, SQLSelectUserQuery, username)
	if err != nil {
		return false, errors.Wrap(err)
	}
	defer rows.Close()

	return rows.Next(), nil
}

func (m *MySQLConfigurator) DropUser(ctx context.Context, username string) error {
	stmt, err := SQLDropUserQuery.PrepareSanitized(UserDefinedIdentifier(username))
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
		if err := m.applyDatabasePermissions(ctx, username, dbname, dbResources); err != nil {
			return errors.Wrap(err)
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
	rows, err := m.db.QueryContext(ctx, SQLSelectDatabasesWithUserPrivilegesQuery, username, username, username)
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

func (m *MySQLConfigurator) applyDatabasePermissions(ctx context.Context, username string, dbname string, dbResources []otterizev1alpha3.DatabaseResource) error {
	for _, dbResource := range dbResources {
		if err := m.applyDatabaseResourcePermissions(ctx, username, dbname, dbResource); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (m *MySQLConfigurator) applyDatabaseResourcePermissions(ctx context.Context, username string, dbname string, dbResource otterizev1alpha3.DatabaseResource) error {
	// TODO: diff and revoke excess permissions

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
	stmt, err := SQLGrantAllPermissionsOnDatabaseStatement.PrepareSanitized(UserDefinedIdentifier(dbname), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) grantPermissionsOnDatabase(ctx context.Context, username string, dbname string, operations []otterizev1alpha3.DatabaseOperation) error {
	stmt, err := SQLGrantPermissionsOnDatabaseStatement.PrepareSanitized(operations, UserDefinedIdentifier(dbname), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) grantAllPermissionsOnTable(ctx context.Context, username string, dbname string, tableName string) error {
	stmt, err := SQLGrantAllPermissionsOnTableStatement.PrepareSanitized(UserDefinedIdentifier(dbname), UserDefinedIdentifier(tableName), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) grantPermissionsOnTable(ctx context.Context, username string, dbname string, tableName string, operations []otterizev1alpha3.DatabaseOperation) error {
	stmt, err := SQLGrantPermissionsOnTableStatement.PrepareSanitized(operations, UserDefinedIdentifier(dbname), UserDefinedIdentifier(tableName), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) revokeTablePermissions(ctx context.Context, username string, table sqlutils.SQLTableIdentifier) error {
	stmt, err := SQLRevokeAllPermissionsOnTableStatement.PrepareSanitized(UserDefinedIdentifier(table.TableSchema), UserDefinedIdentifier(table.TableName), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) revokeDatabasePermissions(ctx context.Context, username string, dbname string) error {
	stmt, err := SQLRevokeAllPermissionsOnDatabaseStatement.PrepareSanitized(UserDefinedIdentifier(dbname), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) RevokeAllDatabasePermissionsForUser(ctx context.Context, pgUsername string) error {
	stmt, err := SQLRevokeAllPermissionsForUserStatement.PrepareSanitized(UserDefinedIdentifier(pgUsername))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) Close() {
	if m.db == nil {
		return
	}

	if err := m.db.Close(); err != nil {
		// Intentionally no error returned - clean up error
		logrus.Errorf("Failed closing connection to: %s", m.databaseInfo.Address)
	}
}
