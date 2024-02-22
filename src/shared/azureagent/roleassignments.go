package azureagent

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/amit7itz/goset"
	"github.com/google/uuid"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
)

func (a *Agent) createRoleAssignment(ctx context.Context, scope string, userAssignedIdentity armmsi.Identity, roleDefinition armauthorization.RoleDefinition) error {
	roleAssignmentName := uuid.New().String()
	roleAssignment, err := a.roleAssignmentsClient.Create(
		ctx,
		scope,
		roleAssignmentName,
		armauthorization.RoleAssignmentCreateParameters{
			Properties: &armauthorization.RoleAssignmentProperties{
				PrincipalID:      userAssignedIdentity.Properties.PrincipalID,
				PrincipalType:    lo.ToPtr(armauthorization.PrincipalTypeServicePrincipal),
				RoleDefinitionID: roleDefinition.ID,
			},
		},
		nil)
	if err != nil {
		return errors.Wrap(err)
	}
	logrus.WithField("scope", *roleAssignment.Properties.Scope).
		WithField("role", *roleAssignment.Properties.RoleDefinitionID).
		WithField("assignment", *roleAssignment.Name).
		Debug("role assignment created")
	return nil
}

func (a *Agent) deleteRoleAssignment(ctx context.Context, roleAssignment armauthorization.RoleAssignment) error {
	logrus.WithField("scope", *roleAssignment.Properties.Scope).
		WithField("role", *roleAssignment.Properties.RoleDefinitionID).
		WithField("assignment", *roleAssignment.Name).
		Debug("deleting role assignment")
	_, err := a.roleAssignmentsClient.Delete(ctx, *roleAssignment.Properties.Scope, *roleAssignment.Name, nil)
	if err != nil {
		return errors.Wrap(err)
	}
	return nil
}

func (a *Agent) listRoleAssignments(ctx context.Context, userAssignedIdentity armmsi.Identity) ([]armauthorization.RoleAssignment, error) {
	var roleAssignments []armauthorization.RoleAssignment
	pager := a.roleAssignmentsClient.NewListForSubscriptionPager(
		&armauthorization.RoleAssignmentsClientListForSubscriptionOptions{
			Filter: lo.ToPtr(fmt.Sprintf("principalId eq {%s}", *userAssignedIdentity.Properties.PrincipalID)),
		})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrap(err)
		}
		for _, roleAssignment := range page.Value {
			roleAssignments = append(roleAssignments, *roleAssignment)
		}
	}

	return roleAssignments, nil
}

func (a *Agent) findRoleDefinitionByName(ctx context.Context, scope string, roleNames []string) (map[string]armauthorization.RoleDefinition, error) {
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
