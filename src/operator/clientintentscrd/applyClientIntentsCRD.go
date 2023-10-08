package clientintentscrd

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
var crdContents []byte

func EnsureClientIntentsCRD(ctx context.Context, k8sClient client.Client, operatorNamespace string) error {
	crdToCreate := apiextensionsv1.CustomResourceDefinition{}
	err := yaml.Unmarshal(crdContents, &crdToCreate)
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

	updatedCRD := crd.DeepCopy()
	updatedCRD.Spec = crdToCreate.Spec
	err = k8sClient.Patch(ctx, updatedCRD, client.MergeFrom(&crd))
	if err != nil {
		return fmt.Errorf("could not Patch ClientIntents CRD: %w", err)
	}

	return nil
}
