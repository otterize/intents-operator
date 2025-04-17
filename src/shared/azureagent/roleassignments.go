package azureagent

import (
	"context"
	azureerrors "github.com/Azure/azure-sdk-for-go-extensions/pkg/errors"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/amit7itz/goset"
	"github.com/google/uuid"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"strings"
)

const (
	OtterizeRoleAssignmentTag = "OtterizeCustomRole"
)

func (a *Agent) IsCustomRoleAssignment(roleAssignment armauthorization.RoleAssignment) bool {
	if roleAssignment.Properties.Description == nil {
		return false
	}
	return *roleAssignment.Properties.Description == OtterizeRoleAssignmentTag
}

func (a *Agent) CreateRoleAssignment(ctx context.Context, scope string, userAssignedIdentity armmsi.Identity, roleDefinition armauthorization.RoleDefinition, desc *string) error {
	roleAssignmentName := uuid.NewString()

	_, err := a.roleAssignmentsClient.Create(
		ctx,
		scope,
		roleAssignmentName,
		armauthorization.RoleAssignmentCreateParameters{
			Properties: &armauthorization.RoleAssignmentProperties{
				PrincipalID:      userAssignedIdentity.Properties.PrincipalID,
				PrincipalType:    lo.ToPtr(armauthorization.PrincipalTypeServicePrincipal),
				RoleDefinitionID: roleDefinition.ID,
				Description:      desc,
			},
		},
		nil)

	if err != nil {
		azureErr := azureerrors.IsResponseError(err)
		if azureErr != nil {
			if azureErr.StatusCode == 409 {
				// WA for "RoleAssignmentExists" error which we ignore
				return nil
			}
		}
		return errors.Wrap(err)
	}
	return nil
}

func (a *Agent) DeleteRoleAssignment(ctx context.Context, roleAssignment armauthorization.RoleAssignment) error {
	_, err := a.roleAssignmentsClient.Delete(ctx, *roleAssignment.Properties.Scope, *roleAssignment.Name, nil)
	if err != nil {
		return errors.Wrap(err)
	}

	// If the assignment is for a custom role, we also need to delete the role itself
	if a.IsCustomRoleAssignment(roleAssignment) {
		// The role id is the last hash of the assignments roleDefinitionID
		fullID := *roleAssignment.Properties.RoleDefinitionID
		roleDefinitionID := fullID[strings.LastIndex(fullID, "/")+1:]

		if err := a.DeleteCustomRole(ctx, *roleAssignment.Properties.Scope, roleDefinitionID); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (a *Agent) ListRoleAssignmentsAcrossSubscriptions(ctx context.Context, userAssignedIdentity *armmsi.Identity) ([]armauthorization.RoleAssignment, error) {
	subscriptions, err := a.ListSubscriptions(ctx)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	var roleAssignments []armauthorization.RoleAssignment
	for _, sub := range subscriptions {
		roleAssignmentsForSubscription, err := a.ListRoleAssignmentsForSubscription(ctx, *sub.SubscriptionID, userAssignedIdentity)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		roleAssignments = append(roleAssignments, roleAssignmentsForSubscription...)
	}

	return roleAssignments, nil
}

func (a *Agent) ListRoleAssignmentsForSubscription(ctx context.Context, subscriptionID string, userAssignedIdentity *armmsi.Identity) ([]armauthorization.RoleAssignment, error) {
	roleClient, err := a.GetRoleAssignmentClientForSubscription(subscriptionID)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	var roleAssignments []armauthorization.RoleAssignment
	pager := roleClient.NewListForSubscriptionPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		for _, roleAssignment := range page.Value {
			// We want to list only otterize role assignments
			if !a.IsCustomRoleAssignment(*roleAssignment) {
				continue
			}

			// Skip filtering if userAssignedIdentity is nil
			if userAssignedIdentity == nil {
				roleAssignments = append(roleAssignments, *roleAssignment)
				continue
			}

			if *roleAssignment.Properties.PrincipalID == *userAssignedIdentity.Properties.PrincipalID {
				roleAssignments = append(roleAssignments, *roleAssignment)
			}
		}
	}

	return roleAssignments, nil
}

func (a *Agent) FindRoleDefinitionByName(ctx context.Context, scope string, roleNames []string) (map[string]armauthorization.RoleDefinition, error) {
	roleDefinitionsByName := map[string]armauthorization.RoleDefinition{}
	roleDefinitions := a.roleDefinitionsClient.NewListPager(scope, nil)
	roleNamesSet := goset.FromSlice(roleNames)
	for roleDefinitions.More() {
		page, err := roleDefinitions.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, roleDef := range page.Value {
			roleName := *roleDef.Properties.RoleName
			if roleNamesSet.Contains(roleName) {
				roleDefinitionsByName[roleName] = *roleDef
			}
		}
	}

	missingRoles := lo.Filter(roleNames, func(roleName string, _ int) bool {
		_, exists := roleDefinitionsByName[roleName]
		return !exists
	})
	if len(missingRoles) > 0 {
		return nil, errors.Errorf("azure role definitions not found: %s", missingRoles)
	}

	return roleDefinitionsByName, nil
}
