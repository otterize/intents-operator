package azureagent

import (
	"context"
	"fmt"
	azureerrors "github.com/Azure/azure-sdk-for-go-extensions/pkg/errors"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/google/uuid"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/agentutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
)

const (
	// maxRoleNameLength rules: 3-512 characters
	maxRoleNameLength = 200

	OtterizeCustomRoleTag         = "OtterizeCustomRole"
	OtterizeCustomRoleDescription = "This custom role was created by the Otterize intents-operator's Azure integration. For more details, go to https://otterize.com"
)

func (a *Agent) getCustomRoleScope() string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", a.Conf.SubscriptionID, a.Conf.ResourceGroup)
}

func (a *Agent) GenerateCustomRoleName(uai armmsi.Identity, scope string) string {
	fullName := fmt.Sprintf("%s-%s", *uai.Name, scope)
	return agentutils.TruncateHashName(fullName, maxRoleNameLength)
}

func (a *Agent) ValidateScope(ctx context.Context, scope string) error {
	res, err := a.resourceClient.GetByID(ctx, scope, "2022-09-01", nil)
	if err != nil {
		return err
	} else if res.GenericResource.ID == nil {
		return errors.Errorf("scope %s not found", scope)
	}
	return nil
}

func (a *Agent) CreateCustomRole(ctx context.Context, scope string, uai armmsi.Identity, actions []v2alpha1.AzureAction, dataActions []v2alpha1.AzureDataAction) (*armauthorization.RoleDefinition, error) {
	roleScope := a.getCustomRoleScope()

	formattedActions := lo.Map(actions, func(action v2alpha1.AzureAction, _ int) *string {
		return to.Ptr(string(action))
	})
	formattedDataActions := lo.Map(dataActions, func(action v2alpha1.AzureDataAction, _ int) *string {
		return to.Ptr(string(action))
	})

	id := uuid.NewString()
	name := a.GenerateCustomRoleName(uai, scope)

	roleDefinition := armauthorization.RoleDefinition{
		Properties: &armauthorization.RoleDefinitionProperties{
			RoleName:         to.Ptr(name),
			Description:      to.Ptr(OtterizeCustomRoleDescription),
			AssignableScopes: []*string{to.Ptr(scope)}, // Where the role can be assigned
			Permissions: []*armauthorization.Permission{
				{
					Actions:     formattedActions,
					DataActions: formattedDataActions,
				},
			},
		},
	}

	// create the custom role
	resp, err := a.roleDefinitionsClient.CreateOrUpdate(ctx, roleScope, id, roleDefinition, nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return &resp.RoleDefinition, nil
}

func (a *Agent) UpdateCustomRole(ctx context.Context, role *armauthorization.RoleDefinition, actions []v2alpha1.AzureAction, dataActions []v2alpha1.AzureDataAction) error {
	roleScope := a.getCustomRoleScope()

	formattedActions := lo.Map(actions, func(action v2alpha1.AzureAction, _ int) *string {
		return to.Ptr(string(action))
	})
	formattedDataActions := lo.Map(dataActions, func(action v2alpha1.AzureDataAction, _ int) *string {
		return to.Ptr(string(action))
	})

	role.Properties.Permissions = []*armauthorization.Permission{
		{
			Actions:     formattedActions,
			DataActions: formattedDataActions,
		},
	}

	_, err := a.roleDefinitionsClient.CreateOrUpdate(ctx, roleScope, *role.Name, *role, nil)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) FindCustomRoleByName(ctx context.Context, name string) (*armauthorization.RoleDefinition, bool) {
	scope := a.getCustomRoleScope()
	filter := fmt.Sprintf("roleName eq '%s'", name)

	pager := a.roleDefinitionsClient.NewListPager(scope, &armauthorization.RoleDefinitionsClientListOptions{
		Filter: &filter,
	})

	for pager.More() {
		page, _ := pager.NextPage(ctx)
		for _, role := range page.Value {
			return role, true
		}
	}

	return nil, false
}

func (a *Agent) DeleteCustomRole(ctx context.Context, roleDefinitionID string) error {
	scope := a.getCustomRoleScope()

	_, err := a.roleDefinitionsClient.Delete(ctx, scope, roleDefinitionID, nil)
	if err != nil {
		if azureerrors.IsNotFoundErr(err) {
			return nil
		}
		return errors.Wrap(err)
	}

	return nil
}
