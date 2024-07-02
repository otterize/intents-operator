package webhooks

import (
	"context"
	"fmt"
	otterizev1 "github.com/otterize/intents-operator/src/operator/api/v1"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Shared functions between database server configs
// Database instance names should be unique in a cluster even across different server config types (MySQL, PostgreSQL)

type DatabaseType string

const (
	MySQL    DatabaseType = "mysql"
	Postgres DatabaseType = "postgres"
)

type DatabaseServerConfig struct {
	Name      string
	Namespace string
	Type      DatabaseType
}

func buildAllConfigsList(ctx context.Context, c client.Client) ([]DatabaseServerConfig, error) {
	pgServerConfList := otterizev2alpha1.PostgreSQLServerConfigList{}
	if err := c.List(ctx, &pgServerConfList); err != nil {
		return nil, errors.Wrap(err)
	}
	postgresConfigs := lo.Map(pgServerConfList.Items, func(conf otterizev2alpha1.PostgreSQLServerConfig, _ int) DatabaseServerConfig {
		return DatabaseServerConfig{
			Namespace: conf.Namespace,
			Name:      conf.Name,
			Type:      Postgres,
		}
	})

	mysqlServerConfList := otterizev2alpha1.MySQLServerConfigList{}
	if err := c.List(ctx, &mysqlServerConfList); err != nil {
		return nil, errors.Wrap(err)
	}
	mysqlConfigs := lo.Map(mysqlServerConfList.Items, func(conf otterizev2alpha1.MySQLServerConfig, _ int) DatabaseServerConfig {
		return DatabaseServerConfig{
			Namespace: conf.Namespace,
			Name:      conf.Name,
			Type:      MySQL,
		}
	})

	return append(postgresConfigs, mysqlConfigs...), nil
}

func validateNoDuplicateForCreate(ctx context.Context, client client.Client, configName string) error {
	allConfigs, err := buildAllConfigsList(ctx, client)
	if err != nil {
		return errors.Wrap(err)
	}

	duplicateConf, forbidden := lo.Find(allConfigs, func(existingConf DatabaseServerConfig) bool {
		return configName == existingConf.Name
	})

	if forbidden {
		return &field.Error{
			Type:     field.ErrorTypeDuplicate,
			Field:    "name",
			BadValue: configName,
			Detail: fmt.Sprintf(
				"A database server config already exists with name %s. Existing resource: %s.%s",
				configName, duplicateConf.Name, duplicateConf.Namespace),
		}
	}

	return nil
}

func validateNoDuplicateForUpdate(ctx context.Context, client client.Client, config DatabaseServerConfig) error {
	allConfigs, err := buildAllConfigsList(ctx, client)
	if err != nil {
		return errors.Wrap(err)
	}

	duplicateConf, forbidden := lo.Find(allConfigs, func(existingConf DatabaseServerConfig) bool {
		// We deny admissions for 'UPDATE' if:
		// 1. A config with the same name exists and is not in the same namespace
		// 2. A config with the same name exists and is in the same namespace, but types are not equal
		if config.Name == existingConf.Name {
			return config.Namespace != existingConf.Namespace || config.Type != existingConf.Type
		}
		return false
	})

	if forbidden {
		return &field.Error{
			Type:     field.ErrorTypeDuplicate,
			Field:    "name",
			BadValue: config.Name,
			Detail: fmt.Sprintf(
				"A database server config already exists with name %s. Existing resource: %s.%s",
				config.Name, duplicateConf.Name, duplicateConf.Namespace),
		}
	}

	return nil
}

func validateCredentialsNotEmpty(credentials otterizev2alpha1.DatabaseCredentials) *field.Error {
	if (credentials.Username == "" || credentials.Password == "") && credentials.SecretRef == nil {
		return &field.Error{
			Type:     field.ErrorTypeRequired,
			Field:    "credentials",
			BadValue: credentials,
			Detail:   "Either username and password must be provided or a secretRef must be provided",
		}
	}

	return nil
}

func validateCredentialsNotEmptyV1alpha3(credentials otterizev1alpha3.DatabaseCredentials) *field.Error {
	if (credentials.Username == "" || credentials.Password == "") && credentials.SecretRef == nil {
		return &field.Error{
			Type:     field.ErrorTypeRequired,
			Field:    "credentials",
			BadValue: credentials,
			Detail:   "Either username and password must be provided or a secretRef must be provided",
		}
	}

	return nil
}

func validateCredentialsNotEmptyV1(credentials otterizev1.DatabaseCredentials) *field.Error {
	if (credentials.Username == "" || credentials.Password == "") && credentials.SecretRef == nil {
		return &field.Error{
			Type:     field.ErrorTypeRequired,
			Field:    "credentials",
			BadValue: credentials,
			Detail:   "Either username and password must be provided or a secretRef must be provided",
		}
	}

	return nil
}
