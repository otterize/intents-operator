package azureagent

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/google/uuid"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/agentutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"strings"
)

const (
	// maxRoleNameLength rules: 3-512 characters
	maxRoleNameLength = 200

	AzureCustomRole = "CustomRole"

	OtterizeCustomRoleTag         = "Otterize"
	OtterizeCustomRoleDescription = "This custom role was created by the Otterize intents-operator's Azure integration. For more details, go to https://otterize.com"
)

func (a *Agent) GetSubscriptionScope(scope string) string {
	subscriptionId := strings.Split(scope, "/")[2]
	return fmt.Sprintf("/subscriptions/%s", subscriptionId)
}

func (a *Agent) GenerateCustomRoleName(intents v2alpha1.ClientIntents, scope string) string {
	fullName := fmt.Sprintf("ottr-%s-%s-%s-%s", intents.GetWorkloadName(), intents.Namespace, a.Conf.AKSClusterName, scope)
	return agentutils.TruncateHashName(fullName, maxRoleNameLength)
}

func (a *Agent) CreateCustomRole(ctx context.Context, scope string, clientIntents v2alpha1.ClientIntents, actions []v2alpha1.AzureAction, dataActions []v2alpha1.AzureDataAction) (*armauthorization.RoleDefinition, error) {
	roleScope := a.GetSubscriptionScope(scope)

	formattedActions := lo.Map(actions, func(action v2alpha1.AzureAction, _ int) *string {
		return to.Ptr(string(action))
	})
	formattedDataActions := lo.Map(dataActions, func(action v2alpha1.AzureDataAction, _ int) *string {
		return to.Ptr(string(action))
	})

	id := uuid.NewString()
	name := a.GenerateCustomRoleName(clientIntents, scope)
	description := fmt.Sprintf("%s - %s", OtterizeCustomRoleTag, OtterizeCustomRoleDescription)

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

	logrus.WithField("name", name).Debug("Creating custom role")
	resp, err := a.roleDefinitionsClient.CreateOrUpdate(ctx, roleScope, id, roleDefinition, nil)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return &resp.RoleDefinition, nil
}

func (a *Agent) UpdateCustomRole(ctx context.Context, scope string, role *armauthorization.RoleDefinition, actions []v2alpha1.AzureAction, dataActions []v2alpha1.AzureDataAction) error {
	if role == nil || role.Properties == nil || role.Properties.Permissions == nil || len(role.Properties.Permissions) == 0 {
		return errors.Errorf("role definition is nil or does not have any permissions")
	}

	roleScope := a.GetSubscriptionScope(scope)

	formattedActions := lo.Map(actions, func(action v2alpha1.AzureAction, _ int) *string {
		return to.Ptr(string(action))
	})
	formattedDataActions := lo.Map(dataActions, func(action v2alpha1.AzureDataAction, _ int) *string {
		return to.Ptr(string(action))
	})

	// Compare the actions and dataActions to the existing role definition
	if IsEqualAzureActions(role.Properties.Permissions[0].Actions, formattedActions) && IsEqualAzureActions(role.Properties.Permissions[0].DataActions, formattedDataActions) {
		logrus.Debugf("Role %s already has the correct permissions", *role.Name)
		return nil
	}

	role.Properties.Permissions = []*armauthorization.Permission{
		{
			Actions:     formattedActions,
			DataActions: formattedDataActions,
		},
	}

	logrus.WithField("name", *role.Name).Debug("Updating custom role")
	_, err := a.roleDefinitionsClient.CreateOrUpdate(ctx, roleScope, *role.Name, *role, nil)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) FindCustomRoleByName(ctx context.Context, scope string, name string) (*armauthorization.RoleDefinition, bool) {
	roleScope := a.GetSubscriptionScope(scope)
	filter := fmt.Sprintf("roleName eq '%s'", name)

	pager := a.roleDefinitionsClient.NewListPager(roleScope, &armauthorization.RoleDefinitionsClientListOptions{
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

func (a *Agent) DeleteCustomRole(ctx context.Context, scope string, clientIntents v2alpha1.ClientIntents) error {
	roleScope := a.GetSubscriptionScope(scope)
	roleName := a.GenerateCustomRoleName(clientIntents, scope)
	roles, err := a.FindRoleDefinitionByName(ctx, roleScope, []string{roleName})
	if err != nil {
		return errors.Wrap(err)
	}

	if len(roles) == 0 {
		logrus.WithField("name", roleName).Debug("Custom role not found")
		return nil
	}

	if len(roles) > 1 {
		return errors.Errorf("multiple custom roles found with name %s", roleName)
	}

	role := lo.Values(roles)[0]

	logrus.WithField("id", role.ID).Debug("Deleting custom role")
	_, err = a.roleDefinitionsClient.Delete(ctx, roleScope, *role.Name, nil)
	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) ListCustomRolesForSubscription(ctx context.Context, subscriptionID string) ([]armauthorization.RoleDefinition, error) {
	var customRoles []armauthorization.RoleDefinition

	pager := a.roleDefinitionsClient.NewListPager(subscriptionID, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		for _, role := range page.Value {
			isCustomRole := role.Properties.RoleType != nil && *role.Properties.RoleType == AzureCustomRole
			isOtterizeRole := role.Properties.Description != nil && strings.Contains(*role.Properties.Description, OtterizeCustomRoleTag)

			if isCustomRole && isOtterizeRole {
				customRoles = append(customRoles, *role)
			}
		}
	}

	return customRoles, nil
}
