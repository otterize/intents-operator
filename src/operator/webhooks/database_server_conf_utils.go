package webhooks

import (
	"context"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Shared functions between database server configs
// Database instance names should be unique in a cluster even across different server config types (MySQL, PostgreSQL)

func ValidateNoDuplicateConfNames(ctx context.Context, c client.Client, currName string) error {
	allConfigs, err := buildAllConfigsList(ctx, c)
	if err != nil {
		return errors.Wrap(err)
	}

	conf, found := lo.Find(allConfigs, func(conf types.NamespacedName) bool {
		return currName == conf.Name
	})

	if found {
		return &field.Error{
			Type:     field.ErrorTypeDuplicate,
			Field:    "name",
			BadValue: currName,
			Detail: fmt.Sprintf(
				"A database server config already exists with name %s. Existing resource: %s.%s",
				currName, conf.Name, conf.Namespace),
		}
	}

	return nil
}

func buildAllConfigsList(ctx context.Context, c client.Client) ([]types.NamespacedName, error) {
	pgServerConfList := otterizev1alpha3.PostgreSQLServerConfigList{}
	if err := c.List(ctx, &pgServerConfList); err != nil {
		return nil, errors.Wrap(err)
	}
	postgresConfigs := lo.Map(pgServerConfList.Items, func(conf otterizev1alpha3.PostgreSQLServerConfig, _ int) types.NamespacedName {
		return types.NamespacedName{
			Namespace: conf.Namespace,
			Name:      conf.Name,
		}
	})

	mysqlServerConfList := otterizev1alpha3.MySQLServerConfigList{}
	if err := c.List(ctx, &mysqlServerConfList); err != nil {
		return nil, errors.Wrap(err)
	}
	mysqlConfigs := lo.Map(mysqlServerConfList.Items, func(conf otterizev1alpha3.MySQLServerConfig, _ int) types.NamespacedName {
		return types.NamespacedName{
			Namespace: conf.Namespace,
			Name:      conf.Name,
		}
	})

	return append(postgresConfigs, mysqlConfigs...), nil
}
