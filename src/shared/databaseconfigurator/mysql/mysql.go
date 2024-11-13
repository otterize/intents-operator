package mysql

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	otterizev2 "github.com/otterize/intents-operator/src/operator/api/v2"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator"
	"github.com/otterize/intents-operator/src/shared/databaseconfigurator/sqlutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	sqldblogger "github.com/simukti/sqldb-logger"
	"github.com/simukti/sqldb-logger/logadapter/logrusadapter"
	"github.com/sirupsen/logrus"
	"net/url"
	"strings"
	"sync"
)

const (
	SQLCreateUserStatement        SQLSprintfStatement = "CREATE USER %s IDENTIFIED BY %s"
	SQLAlterUserPasswordStatement SQLSprintfStatement = "ALTER USER IF EXISTS %s IDENTIFIED BY %s"
	SQLDropUserQuery              SQLSprintfStatement = "DROP USER %s"

	SQLGrantPrivilegesOnDatabaseStatement    SQLSprintfStatement = "GRANT %s ON %s.* TO '%s'"
	SQLGrantAllPrivilegesOnDatabaseStatement SQLSprintfStatement = "GRANT ALL ON %s.* TO '%s'"
	SQLGrantPrivilegesOnTableStatement       SQLSprintfStatement = "GRANT %s ON %s.%s TO '%s'"
	SQLGrantAllPrivilegesOnTableStatement    SQLSprintfStatement = "GRANT ALL ON %s.%s TO '%s'"

	SQLRevokeAllPrivilegesForUserStatement    SQLSprintfStatement = "REVOKE ALL ON *.* FROM '%s'"
	SQLRevokeAllPrivilegesOnDatabaseStatement SQLSprintfStatement = "REVOKE ALL ON %s.* FROM '%s'"
	SQLRevokePrivilegesOnDatabaseStatements   SQLSprintfStatement = "REVOKE %s ON %s.* FROM '%s'"
	SQLRevokeAllPrivilegesOnTableStatement    SQLSprintfStatement = "REVOKE ALL ON %s.%s FROM '%s'"
	SQLRevokePrivilegesOnTableStatement       SQLSprintfStatement = "REVOKE %s ON %s.%s FROM '%s'"

	SQLSelectUserQuery = "SELECT 1 FROM mysql.user WHERE user = ?"

	SQLSelectUserDBPrivilegesQuery = "SELECT * FROM mysql.db WHERE user = ?"
	SQLSelectUserTablePrivileges   = "SELECT Db,Table_name,Table_priv FROM mysql.tables_priv WHERE user = ?"

	DefaultDatabase = "mysql"
)

func privilegesToString(privs []PrivilegeType) []string {
	return lo.Map(privs, func(priv PrivilegeType, _ int) string {
		return priv.String()
	})
}

type MySQLDatabaseInfo struct {
	Address     string
	Credentials databaseconfigurator.DatabaseCredentials
}

type DBUserPrivilege struct {
	Db         string
	Privileges []PrivilegeType
}

type TablePrivilege struct {
	Db         string
	TableName  string
	Privileges []PrivilegeType
}

type MySQLConfigurator struct {
	databaseInfo MySQLDatabaseInfo
	db           *sql.DB
	setDBMutex   sync.Mutex
	logger       *logrus.Entry
}

func NewMySQLConfigurator(ctx context.Context, databaseInfo MySQLDatabaseInfo) (*MySQLConfigurator, error) {
	m := &MySQLConfigurator{
		databaseInfo: databaseInfo,
		setDBMutex:   sync.Mutex{},
		logger:       logrus.WithField("database", databaseInfo.Address),
	}

	if err := m.setConnection(DefaultDatabase); err != nil {
		return nil, errors.Wrap(err)
	}

	if err := m.db.PingContext(ctx); err != nil {
		defer m.CloseConnection(ctx)
		return nil, errors.Wrap(err)
	}

	return m, nil
}

func (m *MySQLConfigurator) dbOperationsToPrivileges(dbOperations []otterizev2.DatabaseOperation) []PrivilegeType {
	return lo.Map(dbOperations, func(op otterizev2.DatabaseOperation, _ int) PrivilegeType {
		switch op {
		case otterizev2.DatabaseOperationSelect:
			return SelectPriv
		case otterizev2.DatabaseOperationInsert:
			return InsertPriv
		case otterizev2.DatabaseOperationUpdate:
			return UpdatePriv
		case otterizev2.DatabaseOperationDelete:
			return DeletePriv
		default:
			m.logger.WithField("operation", op).Error("Unexpected DB operation, will grant usage only privileges")
			return UsagePriv
		}
	})
}

func (m *MySQLConfigurator) formatConnectionString(databaseName string) string {
	return fmt.Sprintf(
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
	m.logger.WithField("username", username).Info("Creating user")
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
	m.logger.WithField("username", username).Info("Dropping user")
	stmt, err := SQLDropUserQuery.PrepareSanitized(UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}

	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) AlterUserPassword(ctx context.Context, username string, password string) error {
	stmt, err := SQLAlterUserPasswordStatement.PrepareSanitized(UserDefinedIdentifier(username), NonUserInputString(password))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) ApplyDatabasePermissionsForUser(ctx context.Context, username string, dbnameToSQLPermissionss map[string][]otterizev2.SQLPrivileges) error {
	// apply new intents
	for dbname, dbResources := range dbnameToSQLPermissionss {
		if err := m.applyDatabasePermissions(ctx, username, dbname, dbResources); err != nil {
			return errors.Wrap(err)
		}
	}

	if err := m.revokeExcessDBLevelPrivileges(ctx, username, dbnameToSQLPermissionss); err != nil {
		return errors.Wrap(err)
	}

	if err := m.revokeExcessTableLevelPrivileges(ctx, username, dbnameToSQLPermissionss); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) applyDatabasePermissions(ctx context.Context, username string, dbname string, dbResources []otterizev2.SQLPrivileges) error {
	for _, dbResource := range dbResources {
		if err := m.applySQLPermissionsPermissions(ctx, username, dbname, dbResource); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (m *MySQLConfigurator) applySQLPermissionsPermissions(ctx context.Context, username string, dbname string, dbResource otterizev2.SQLPrivileges) error {
	operations := sqlutils.GetGrantOperations(dbResource.Operations)
	containsAllOperation := sqlutils.ContainsAllOperations(operations)
	applyOnAllTables := dbResource.Table == ""
	if containsAllOperation {
		if applyOnAllTables {
			return m.grantAllPrivilegesOnDatabase(ctx, username, dbname)
		} else {
			return m.grantAllPrivilegesOnTable(ctx, username, dbname, dbResource.Table)
		}
	}

	privileges := m.dbOperationsToPrivileges(operations)
	if applyOnAllTables {
		// Grant specified privileges on database level
		return m.grantPrivilegesOnDatabase(ctx, username, dbname, privileges)
	} else {
		// Grant specified privileges on table level
		return m.grantPrivilegesOnTable(ctx, username, dbname, dbResource.Table, privileges)
	}
}

func (m *MySQLConfigurator) dbRowToDBPrivilege(rows *sql.Rows, columns []string) (DBUserPrivilege, error) {
	var p DBUserPrivilege

	vals := make([]any, len(columns))
	for i, _ := range columns {
		vals[i] = new(string)
	}

	if err := rows.Scan(vals...); err != nil {
		return p, errors.Wrap(err)
	}

	valuesAsMap := lo.FromPairs(lo.Map(vals, func(val any, i int) lo.Entry[string, string] {
		return lo.Entry[string, string]{Key: columns[i], Value: *(val.(*string))}
	}))

	p.Db = valuesAsMap["Db"]

	privColumns := lo.Keys(Col2PrivType)
	enabledPrivColumns := lo.Filter(privColumns, func(col string, _ int) bool {
		return valuesAsMap[col] == "Y"
	})

	for _, col := range enabledPrivColumns {
		priv, ok := NewPrivFromColumn(col)
		if !ok {
			return p, errors.Errorf("unexpected privilege column: %s", col)
		}
		p.Privileges = append(p.Privileges, priv)
	}

	return p, nil
}

func (m *MySQLConfigurator) queryUserDBPrivileges(ctx context.Context, username string) ([]DBUserPrivilege, error) {
	rows, err := m.db.QueryContext(ctx, SQLSelectUserDBPrivilegesQuery, username)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	defer rows.Close()

	userPrivileges := make([]DBUserPrivilege, 0)

	columns, err := rows.Columns()
	if err != nil {
		return nil, errors.Wrap(err)
	}

	for rows.Next() {
		p, err := m.dbRowToDBPrivilege(rows, columns)
		if err != nil {
			return nil, errors.Wrap(err)
		}
		userPrivileges = append(userPrivileges, p)
	}

	return userPrivileges, nil
}

func (m *MySQLConfigurator) getDBLevelPrivilegesToRevoke(userDBPrivileges DBUserPrivilege, dbnameToSQLPermissionss map[string][]otterizev2.SQLPrivileges) []PrivilegeType {
	dbResources, ok := dbnameToSQLPermissionss[userDBPrivileges.Db]
	if !ok {
		// DB not specified in applied intents, should revoke all privileges
		return []PrivilegeType{AllPriv}
	}

	applyOnAllTablesResources := lo.Filter(dbResources, func(dbResource otterizev2.SQLPrivileges, _ int) bool {
		return dbResource.Table == ""
	})
	if len(applyOnAllTablesResources) == 0 {
		// no DB level intents applied, should revoke all DB-level privileges
		return []PrivilegeType{AllPriv}
	}

	intentsAllowedOperations := lo.Flatten(lo.Map(applyOnAllTablesResources, func(dbResource otterizev2.SQLPrivileges, _ int) []otterizev2.DatabaseOperation {
		return sqlutils.GetGrantOperations(dbResource.Operations)
	}))

	if sqlutils.ContainsAllOperations(intentsAllowedOperations) {
		// all operations are allowed on DB level, should not revoke any privileges
		return nil
	}

	intentsAllowedPrivileges := m.dbOperationsToPrivileges(intentsAllowedOperations)
	return lo.Without(userDBPrivileges.Privileges, intentsAllowedPrivileges...)
}

func (m *MySQLConfigurator) revokeExcessDBLevelPrivileges(ctx context.Context, username string, dbnameToSQLPermissionss map[string][]otterizev2.SQLPrivileges) error {
	userDBPrivileges, err := m.queryUserDBPrivileges(ctx, username)
	if err != nil {
		return errors.Wrap(err)
	}

	for _, dbPrivilege := range userDBPrivileges {
		privilegesToRevoke := m.getDBLevelPrivilegesToRevoke(dbPrivilege, dbnameToSQLPermissionss)
		if lo.Contains(privilegesToRevoke, AllPriv) {
			if err := m.revokeAllDatabasePrivileges(ctx, username, dbPrivilege.Db); err != nil {
				return errors.Wrap(err)
			}
		} else if len(privilegesToRevoke) > 0 {
			if err := m.revokeDatabasePrivileges(ctx, username, dbPrivilege.Db, privilegesToRevoke); err != nil {
				return errors.Wrap(err)
			}
		}
	}

	return nil
}

func (m *MySQLConfigurator) dbRowToTablePrivilege(row *sql.Rows) (TablePrivilege, error) {
	var p TablePrivilege
	var tablePriv string // as set
	if err := row.Scan(&p.Db, &p.TableName, &tablePriv); err != nil {
		return p, errors.Wrap(err)
	}
	privStrings := strings.Split(tablePriv, ",")
	for _, s := range privStrings {
		s = strings.TrimSpace(s)
		priv, ok := NewPrivFromSetEnum(s)
		if !ok {
			return p, errors.Errorf("unexpected privilege: %s", s)
		}
		p.Privileges = append(p.Privileges, priv)
	}

	return p, nil
}

func (m *MySQLConfigurator) queryUserTablePrivileges(ctx context.Context, username string) ([]TablePrivilege, error) {
	rows, err := m.db.QueryContext(ctx, SQLSelectUserTablePrivileges, username)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	defer rows.Close()

	tablePrivileges := make([]TablePrivilege, 0)
	for rows.Next() {
		p, err := m.dbRowToTablePrivilege(rows)
		if err != nil {
			return nil, errors.Wrap(err)
		}
		tablePrivileges = append(tablePrivileges, p)
	}

	return tablePrivileges, nil
}

func (m *MySQLConfigurator) getTableLevelPrivilegesToRevoke(userTablePrivileges TablePrivilege, dbnameToSQLPermissionss map[string][]otterizev2.SQLPrivileges) []PrivilegeType {
	dbResources, ok := dbnameToSQLPermissionss[userTablePrivileges.Db]
	if !ok {
		// DB not specified in applied intents, should revoke all privileges
		return []PrivilegeType{AllPriv}
	}

	tableResources := lo.Filter(dbResources, func(dbResource otterizev2.SQLPrivileges, _ int) bool {
		return dbResource.Table == userTablePrivileges.TableName
	})

	if len(tableResources) == 0 {
		// No table level intents applied, should revoke all table-specific privileges
		return []PrivilegeType{AllPriv}
	}

	intentsAllowedOperations := lo.Flatten(lo.Map(tableResources, func(dbResource otterizev2.SQLPrivileges, _ int) []otterizev2.DatabaseOperation {
		return sqlutils.GetGrantOperations(dbResource.Operations)
	}))

	if sqlutils.ContainsAllOperations(intentsAllowedOperations) {
		// all operations are allowed on table level, should not revoke any privileges
		return nil
	}

	intentsAllowedPrivileges := m.dbOperationsToPrivileges(intentsAllowedOperations)
	return lo.Without(userTablePrivileges.Privileges, intentsAllowedPrivileges...)
}

func (m *MySQLConfigurator) revokeExcessTableLevelPrivileges(ctx context.Context, username string, dbnameToSQLPermissionss map[string][]otterizev2.SQLPrivileges) error {
	userTablePrivileges, err := m.queryUserTablePrivileges(ctx, username)
	if err != nil {
		return errors.Wrap(err)
	}

	for _, tablePrivilege := range userTablePrivileges {
		privilegesToRevoke := m.getTableLevelPrivilegesToRevoke(tablePrivilege, dbnameToSQLPermissionss)
		if lo.Contains(privilegesToRevoke, AllPriv) {
			if err := m.revokeAllTablePrivileges(ctx, username, sqlutils.SQLTableIdentifier{TableSchema: tablePrivilege.Db, TableName: tablePrivilege.TableName}); err != nil {
				return errors.Wrap(err)
			}
		} else if len(privilegesToRevoke) > 0 {
			if err := m.revokeTablePrivileges(ctx, username, sqlutils.SQLTableIdentifier{TableSchema: tablePrivilege.Db, TableName: tablePrivilege.TableName}, privilegesToRevoke); err != nil {
				return errors.Wrap(err)
			}
		}
	}

	return nil
}

func (m *MySQLConfigurator) grantAllPrivilegesOnDatabase(ctx context.Context, username string, dbname string) error {
	m.logger.WithField("username", username).WithField("dbname", dbname).Debug("Granting all privileges on database")
	stmt, err := SQLGrantAllPrivilegesOnDatabaseStatement.PrepareSanitized(UserDefinedIdentifier(dbname), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) grantPrivilegesOnDatabase(ctx context.Context, username string, dbname string, privileges []PrivilegeType) error {
	m.logger.WithField("username", username).WithField("dbname", dbname).WithField("privileges", privilegesToString(privileges)).Debug("Granting privileges on database")
	stmt, err := SQLGrantPrivilegesOnDatabaseStatement.PrepareSanitized(privileges, UserDefinedIdentifier(dbname), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) grantAllPrivilegesOnTable(ctx context.Context, username string, dbname string, tableName string) error {
	m.logger.WithField("username", username).WithField("dbname", dbname).WithField("tableName", tableName).Debug("Granting all privileges on table")
	stmt, err := SQLGrantAllPrivilegesOnTableStatement.PrepareSanitized(UserDefinedIdentifier(dbname), UserDefinedIdentifier(tableName), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) grantPrivilegesOnTable(ctx context.Context, username string, dbname string, tableName string, privileges []PrivilegeType) error {
	m.logger.WithField("username", username).WithField("dbname", dbname).WithField("tableName", tableName).WithField("privileges", privilegesToString(privileges)).Debug("Granting privileges on table")
	stmt, err := SQLGrantPrivilegesOnTableStatement.PrepareSanitized(privileges, UserDefinedIdentifier(dbname), UserDefinedIdentifier(tableName), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) revokeAllTablePrivileges(ctx context.Context, username string, table sqlutils.SQLTableIdentifier) error {
	m.logger.WithField("username", username).WithField("table", table).Info("Revoking all privileges on table")
	stmt, err := SQLRevokeAllPrivilegesOnTableStatement.PrepareSanitized(UserDefinedIdentifier(table.TableSchema), UserDefinedIdentifier(table.TableName), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) revokeTablePrivileges(ctx context.Context, username string, table sqlutils.SQLTableIdentifier, privileges []PrivilegeType) error {
	m.logger.WithField("username", username).WithField("table", table).WithField("privileges", privilegesToString(privileges)).Info("Revoking privileges on table")
	stmt, err := SQLRevokePrivilegesOnTableStatement.PrepareSanitized(privileges, UserDefinedIdentifier(table.TableSchema), UserDefinedIdentifier(table.TableName), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) revokeAllDatabasePrivileges(ctx context.Context, username string, dbname string) error {
	m.logger.WithField("username", username).WithField("dbname", dbname).Info("Revoking all privileges on database")
	stmt, err := SQLRevokeAllPrivilegesOnDatabaseStatement.PrepareSanitized(UserDefinedIdentifier(dbname), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) revokeDatabasePrivileges(ctx context.Context, username string, dbname string, privileges []PrivilegeType) error {
	m.logger.WithField("username", username).WithField("dbname", dbname).WithField("privileges", privilegesToString(privileges)).Info("Revoking privileges on database")
	stmt, err := SQLRevokePrivilegesOnDatabaseStatements.PrepareSanitized(privileges, UserDefinedIdentifier(dbname), UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil

}

func (m *MySQLConfigurator) revokeAllDatabasePrivilegesForUser(ctx context.Context, username string) error {
	m.logger.WithField("username", username).Info("Revoking all privileges for user")
	stmt, err := SQLRevokeAllPrivilegesForUserStatement.PrepareSanitized(UserDefinedIdentifier(username))
	if err != nil {
		return errors.Wrap(err)
	}
	if _, err := m.db.ExecContext(ctx, stmt); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (m *MySQLConfigurator) RevokeAllDatabasePermissionsForUser(ctx context.Context, username string) error {
	return m.revokeAllDatabasePrivilegesForUser(ctx, username)
}

func (m *MySQLConfigurator) CloseConnection(ctx context.Context) {
	if m.db == nil {
		return
	}

	if err := m.db.Close(); err != nil {
		// Intentionally no error returned - clean up error
		logrus.WithError(err).Errorf("Failed closing connection to: %s", m.databaseInfo.Address)
	}
}
