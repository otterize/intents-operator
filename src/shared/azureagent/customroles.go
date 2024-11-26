package azureagent

import (
	"context"
	"fmt"
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

	otterizeCustomRoleTag = "ocr"
)

func (a *Agent) getCustomRoleScope() string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", a.Conf.SubscriptionID, a.Conf.ResourceGroup)
}

func (a *Agent) GenerateCustomRoleName(uai armmsi.Identity, scope string) string {
	fullName := fmt.Sprintf("%s-%s", *uai.Name, scope)
	return agentutils.TruncateHashName(fullName, maxRoleNameLength)
}

func (a *Agent) CreateCustomRole(ctx context.Context, scope string, uai armmsi.Identity, actions []v2alpha1.AzureAction, dataActions []v2alpha1.AzureDataAction) error {
	roleScope := a.getCustomRoleScope()

	formattedActions := lo.Map(actions, func(action v2alpha1.AzureAction, _ int) *string {
		return to.Ptr(string(action))
	})
	formattedDataActions := lo.Map(dataActions, func(action v2alpha1.AzureDataAction, _ int) *string {
		return to.Ptr(string(action))
	})

	id := uuid.NewString()
	name := a.GenerateCustomRoleName(uai, scope)
	description := fmt.Sprintf("Otterize managed custom role for uai [%s] with permissions for scope [%s]", *uai.Name, scope)

	roleDefinition := armauthorization.RoleDefinition{
		Properties: &armauthorization.RoleDefinitionProperties{
			RoleName:         to.Ptr(name),
			Description:      to.Ptr(description),
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
		return errors.Wrap(err)
	}

	// create a role assignment for the custom role
	err = a.CreateRoleAssignment(ctx, scope, uai, resp.RoleDefinition, to.Ptr(otterizeCustomRoleTag))
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
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
		return errors.Wrap(err)
	}

	return nil
}
