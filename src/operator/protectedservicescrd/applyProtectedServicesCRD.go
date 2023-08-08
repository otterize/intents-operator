package protectedservicescrd

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

//go:embed protectedservices-customresourcedefinition.yaml
var crdContents []byte

func EnsureProtectedServicesCRD(ctx context.Context, client client.Client) error {
	crdToCreate := apiextensionsv1.CustomResourceDefinition{}
	err := yaml.Unmarshal(crdContents, &crdToCreate)
	if err != nil {
		return fmt.Errorf("failed to unmarshal protected services CRD: %w", err)
	}
	crd := apiextensionsv1.CustomResourceDefinition{}
	err = client.Get(ctx, types.NamespacedName{Name: crdToCreate.Name}, &crd)
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("could not get protected services CRD: %w", err)
	}

	if k8serrors.IsNotFound(err) {
		err := client.Create(ctx, &crdToCreate)
		if err != nil {
			return fmt.Errorf("could not create protected services CRD: %w", err)
		}
		return nil
	}

	return nil
}
