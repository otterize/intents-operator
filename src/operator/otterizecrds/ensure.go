package otterizecrds

import (
	"context"
	_ "embed"
	"github.com/otterize/intents-operator/src/shared/errors"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed approvedclientintents-customresourcedefinition.yaml
var approvedClientIntentsCRDContents []byte

//go:embed clientintents-customresourcedefinition.yaml
var clientIntentsCRDContents []byte

//go:embed protectedservices-customresourcedefinition.yaml
var protectedServiceCRDContents []byte

//go:embed kafkaserverconfigs-customresourcedefinition.yaml
var KafkaServerConfigContents []byte

//go:embed postgresqlserverconfigs-customresourcedefinition.yaml
var PostgreSQLServerConfigContents []byte

//go:embed mysqlserverconfigs-customresourcedefinition.yaml
var MySQLServerConfigContents []byte

func Ensure(ctx context.Context, k8sClient client.Client, operatorNamespace string, certPem []byte) error {
	err := ensureCRD(ctx, k8sClient, operatorNamespace, clientIntentsCRDContents, certPem)
	if err != nil {
		return errors.Errorf("failed to ensure CLientIntents CRD: %w", err)
	}
	err = ensureCRD(ctx, k8sClient, operatorNamespace, approvedClientIntentsCRDContents, certPem)
	if err != nil {
		return errors.Errorf("failed to ensure Approved CLientIntents CRD: %w", err)
	}
	err = ensureCRD(ctx, k8sClient, operatorNamespace, protectedServiceCRDContents, certPem)
	if err != nil {
		return errors.Errorf("failed to ensure ProtectedService CRD: %w", err)
	}
	err = ensureCRD(ctx, k8sClient, operatorNamespace, KafkaServerConfigContents, certPem)
	if err != nil {
		return errors.Errorf("failed to ensure KafkaServerConfig CRD: %w", err)
	}
	err = ensureCRD(ctx, k8sClient, operatorNamespace, PostgreSQLServerConfigContents, certPem)
	if err != nil {
		return errors.Errorf("failed to ensure PostgreSQLServerConfig CRD: %w", err)
	}
	err = ensureCRD(ctx, k8sClient, operatorNamespace, MySQLServerConfigContents, certPem)
	if err != nil {
		return errors.Errorf("failed to ensure MySQLServerConfig CRD: %w", err)
	}
	return nil
}

func ensureCRD(ctx context.Context, k8sClient client.Client, operatorNamespace string, crdContent []byte, certPem []byte) error {
	crdToCreate := apiextensionsv1.CustomResourceDefinition{}
	err := yaml.Unmarshal(crdContent, &crdToCreate)
	if err != nil {
		return errors.Errorf("failed to unmarshal ClientIntents CRD: %w", err)
	}

	crdToCreate.Spec.Conversion.Webhook.ClientConfig.Service.Namespace = operatorNamespace
	crdToCreate.Spec.Conversion.Webhook.ClientConfig.CABundle = certPem

	crd := apiextensionsv1.CustomResourceDefinition{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: crdToCreate.Name}, &crd)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Errorf("could not get ClientIntents CRD: %w", err)
	}
	if k8serrors.IsNotFound(err) {
		err := k8sClient.Create(ctx, &crdToCreate)
		if err != nil {
			return errors.Errorf("could not create ClientIntents CRD: %w", err)
		}
		return nil
	}

	// Update CRD
	updatedCRD := crd.DeepCopy()
	for key, value := range crdToCreate.Labels {
		if updatedCRD.Labels == nil {
			updatedCRD.Labels = make(map[string]string)
		}
		updatedCRD.Labels[key] = value
	}
	for key, value := range crdToCreate.Annotations {
		if updatedCRD.Annotations == nil {
			updatedCRD.Annotations = make(map[string]string)
		}
		updatedCRD.Annotations[key] = value
	}
	updatedCRD.Spec = *crdToCreate.Spec.DeepCopy()
	err = k8sClient.Patch(ctx, updatedCRD, client.MergeFrom(&crd))
	if err != nil {
		return errors.Errorf("could not Patch %s CRD: %w", crd.Name, err)
	}

	return nil
}

func GetCRDDefinitionByName(name string) (*apiextensionsv1.CustomResourceDefinition, error) {
	var err error
	crd := apiextensionsv1.CustomResourceDefinition{}
	switch name {
	case "approvedclientintents.k8s.otterize.com":
		err = yaml.Unmarshal(approvedClientIntentsCRDContents, &crd)
	case "clientintents.k8s.otterize.com":
		err = yaml.Unmarshal(clientIntentsCRDContents, &crd)
	case "protectedservices.k8s.otterize.com":
		err = yaml.Unmarshal(protectedServiceCRDContents, &crd)
	case "kafkaserverconfigs.k8s.otterize.com":
		err = yaml.Unmarshal(KafkaServerConfigContents, &crd)
	case "postgresqlserverconfigs.k8s.otterize.com":
		err = yaml.Unmarshal(PostgreSQLServerConfigContents, &crd)
	case "mysqlserverconfigs.k8s.otterize.com":
		err = yaml.Unmarshal(MySQLServerConfigContents, &crd)
	default:
		return nil, errors.Errorf("unknown CRD name: %s", name)
	}

	if err != nil {
		return nil, err
	}
	return &crd, nil
}
