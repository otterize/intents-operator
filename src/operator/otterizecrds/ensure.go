package otterizecrds

import (
	"context"
	_ "embed"
	"fmt"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed clientintents-customresourcedefinition.yaml
var clientIntentsCRDContents []byte

//go:embed protectedservices-customresourcedefinition.yaml
var protectedServiceCRDContents []byte

//go:embed kafkaserverconfigs-customresourcedefinition.yaml
var KafkaServerConfigContents []byte

func Ensure(ctx context.Context, k8sClient client.Client, operatorNamespace string) error {
	err := ensureCRD(ctx, k8sClient, operatorNamespace, clientIntentsCRDContents)
	if err != nil {
		return fmt.Errorf("failed to ensure CLientIntents CRD: %w", err)
	}
	err = ensureCRD(ctx, k8sClient, operatorNamespace, protectedServiceCRDContents)
	if err != nil {
		return fmt.Errorf("failed to ensure ProtectedService CRD: %w", err)
	}
	err = ensureCRD(ctx, k8sClient, operatorNamespace, KafkaServerConfigContents)
	if err != nil {
		return fmt.Errorf("failed to ensure KafkaServerConfig CRD: %w", err)
	}
	return nil
}

func ensureCRD(ctx context.Context, k8sClient client.Client, operatorNamespace string, crdContent []byte) error {
	crdToCreate := apiextensionsv1.CustomResourceDefinition{}
	err := yaml.Unmarshal(crdContent, &crdToCreate)
	if err != nil {
		return fmt.Errorf("failed to unmarshal ClientIntents CRD: %w", err)
	}
	crdToCreate.Spec.Conversion.Webhook.ClientConfig.Service.Namespace = operatorNamespace
	crd := apiextensionsv1.CustomResourceDefinition{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: crdToCreate.Name}, &crd)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("could not get ClientIntents CRD: %w", err)
	}
	if k8serrors.IsNotFound(err) {
		err := k8sClient.Create(ctx, &crdToCreate)
		if err != nil {
			return fmt.Errorf("could not create ClientIntents CRD: %w", err)
		}
		return nil
	}

	return nil
}

func GetCRDDefinitionByName(name string) (*apiextensionsv1.CustomResourceDefinition, error) {
	var err error
	crd := apiextensionsv1.CustomResourceDefinition{}
	switch name {
	case "clientintents.k8s.otterize.com":
		err = yaml.Unmarshal(clientIntentsCRDContents, &crd)
	case "protectedservices.k8s.otterize.com":
		err = yaml.Unmarshal(protectedServiceCRDContents, &crd)
	case "kafkaserverconfigs.k8s.otterize.com":
		err = yaml.Unmarshal(protectedServiceCRDContents, &crd)
	default:
		return nil, fmt.Errorf("unknown CRD name: %s", name)
	}

	if err != nil {
		return nil, err
	}
	return &crd, nil
}
