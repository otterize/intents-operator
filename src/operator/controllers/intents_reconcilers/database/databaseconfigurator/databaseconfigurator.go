package databaseconfigurator

import (
	"context"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
)

type DatabaseConfigurator interface {
	ConfigureDBFromIntents(ctx context.Context, workloadName string, namespace string, intents []otterizev1alpha3.Intent, permissionChange otterizev1alpha3.DBPermissionChange) error
	FormatConnectionString(databaseName string) string
}
