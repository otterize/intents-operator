package databaseconfigurator

import (
	"context"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
)

const (
	DatabaseAccessAnnotation     = "intents.otterize.com/database-access"
	LatestAccessChangeAnnotation = "intents.otterize.com/database-access-update-time"
)

type DatabaseCredentials struct {
	Username string
	Password string
}

type DatabaseConfigurator interface {
	CreateUser(ctx context.Context, username string, password string) error
	ValidateUserExists(ctx context.Context, username string) (bool, error)
	DropUser(ctx context.Context, username string) error
	AlterUserPassword(ctx context.Context, username string, password string) error
	ApplyDatabasePermissionsForUser(ctx context.Context, username string, dbnameToDatabaseResources map[string][]otterizev2alpha1.SQLPermissions) error
	RevokeAllDatabasePermissionsForUser(ctx context.Context, username string) error
	CloseConnection(ctx context.Context)
}
