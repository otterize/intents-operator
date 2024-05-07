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
		"%s:%s@%s/%s",
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
	//TODO implement me
	panic("implement me")
}

func (m *MySQLConfigurator) RevokeAllDatabasePermissionsForUser(ctx context.Context, pgUsername string) error {
	//TODO implement me
	panic("implement me")
}
