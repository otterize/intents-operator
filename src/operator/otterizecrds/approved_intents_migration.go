package otterizecrds

import (
	"context"
	otterizev2beta1 "github.com/otterize/intents-operator/src/operator/api/v2beta1"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

func RunApprovedClientIntentsMigrationIfNeeded(ctx context.Context, client client.Client, operatorNamespace string, certPem []byte) error {
	shouldMigrate, err := isApprovedIntentsMigrationNeeded(ctx, client)
	if err != nil {
		return errors.Wrap(err)
	}
	if !shouldMigrate {
		return nil
	}

	logrus.Info("Performing ApprovedClientIntents migration")

	// make sure we have the approved client intents CRD
	err = ensureCRD(ctx, client, operatorNamespace, approvedClientIntentsCRDContents, certPem)
	if err != nil {
		return errors.Errorf("failed to ensure Approved CLientIntents CRD: %w", err)
	}

	err = createApprovedIntentsForExistingClientIntents(ctx, client)
	if err != nil {
		return errors.Wrap(err)
	}

	logrus.Info("ApprovedClientIntents migration completed")

	return nil

}

func WaitForApprovedClientIntentsMigration(ctx context.Context, client client.Client) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err := waitForMigrationWithContext(timeoutCtx, client)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func waitForMigrationWithContext(ctx context.Context, client client.Client) error {
	for {
		select {
		case <-ctx.Done():
			return errors.New("waiting for migration was cancelled")
		default:
			shouldMigrate, err := isApprovedIntentsMigrationNeeded(ctx, client)
			if err != nil {
				return errors.Wrap(err)
			}
			if !shouldMigrate {
				return nil
			}
		}
	}

}

func createApprovedIntentsForExistingClientIntents(ctx context.Context, client client.Client) error {
	// check every clientIntents has a corresponding approvedClientIntents
	// use v2beta1 because we don't have conversion webhook at this point
	clientIntentsList := &otterizev2beta1.ClientIntentsList{}
	err := client.List(ctx, clientIntentsList)
	if err != nil {
		return errors.Wrap(err)
	}

	for _, clientIntents := range clientIntentsList.Items {
		approvedClientIntents := &otterizev2beta1.ApprovedClientIntents{}
		approvedClientIntents.FromClientIntents(clientIntents)
		if len(clientIntents.Status.ResolvedIPs) > 0 {
			approvedClientIntents.Status.ResolvedIPs = clientIntents.Status.ResolvedIPs
		}
		err = client.Get(ctx, types.NamespacedName{Namespace: approvedClientIntents.Namespace, Name: approvedClientIntents.Name}, &otterizev2beta1.ApprovedClientIntents{})
		if k8serrors.IsNotFound(err) {
			err = client.Create(ctx, approvedClientIntents)
			if err != nil {
				return errors.Wrap(err)
			}
		} else if err != nil {
			return errors.Wrap(err)
		}
	}
	return nil
}

func isApprovedIntentsMigrationNeeded(ctx context.Context, client client.Client) (bool, error) {
	clientIntentsCRD := &apiextensionsv1.CustomResourceDefinition{}
	err := client.Get(ctx, types.NamespacedName{Name: "clientintents.k8s.otterize.com"}, clientIntentsCRD)
	if err != nil {
		return false, errors.Wrap(err)
	}
	v2, ok := lo.Find(clientIntentsCRD.Spec.Versions, func(version apiextensionsv1.CustomResourceDefinitionVersion) bool {
		return strings.HasPrefix(version.Name, "v2")
	})
	if !ok {
		return false, errors.New("no v2 versions found")
	}

	if _, exists := v2.Schema.OpenAPIV3Schema.Properties["status"].Properties["reviewStatus"]; exists {
		// no need to migrate since the CRD already has the reviewStatus field
		return false, nil
	}
	return true, nil
}
