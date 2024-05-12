package databaseconfigurator

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
)

type DatabaseConfigurator interface {
	CreateUser(ctx context.Context, username string, password string) error
	ValidateUserExists(ctx context.Context, username string) (bool, error)
	DropUser(ctx context.Context, username string) error

	ApplyDatabasePermissionsForUser(ctx context.Context, username string, dbnameToDatabaseResources map[string][]otterizev1alpha3.DatabaseResource) error
	RevokeAllDatabasePermissionsForUser(ctx context.Context, username string) error
}
