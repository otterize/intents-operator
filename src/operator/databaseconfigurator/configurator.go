package databaseconfigurator

import (
	"context"
	"github.com/otterize/cloud/src/backend-service/pkg/graph/model"
	"github.com/otterize/cloud/src/backend-service/pkg/lib/apis"
)

type DatabaseConfigurator interface {
	ConfigureDBFromIntents(ctx context.Context, intents []model.IntentInput, permissionChange DBPermissionChange, id apis.OrganizationID) error
}
