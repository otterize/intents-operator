package azurepolicyagent

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/amit7itz/goset"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"strings"
)

type Agent struct {
	*azureagent.Agent
}

func NewAzurePolicyAgent(azureAgent *azureagent.Agent) *Agent {
	return &Agent{azureAgent}
}

func (a *Agent) IntentType() otterizev1alpha3.IntentType {
	return otterizev1alpha3.IntentTypeAzure
}

func (a *Agent) getIntentScope(intent otterizev1alpha3.Intent) (string, error) {
	name := intent.Name
	if !strings.HasPrefix(name, "/") {
		return "", errors.Errorf("expected intent name to start with /, got %s", name)
	}

	if strings.HasPrefix(name, "/subscriptions/") {
		// the name is already a full scope
		return name, nil
	}

	if strings.HasPrefix(name, "/resourceGroups/") {
		// append the subscription ID to the scope
		fullScope := fmt.Sprintf("/subscriptions/%s%s", a.Conf.SubscriptionID, name)
		return fullScope, nil
	}

	// append both the subscription ID and the resource group to the scope
	fullScope := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s%s", a.Conf.SubscriptionID, a.Conf.ResourceGroup, name)
	return fullScope, nil
}

func (a *Agent) AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev1alpha3.Intent) error {
	userAssignedIdentity, err := a.FindUserAssignedIdentity(ctx, namespace, intentsServiceName)
	if err != nil {
		return errors.Wrap(err)
	}

	existingRoleAssignments, err := a.ListRoleAssignments(ctx, userAssignedIdentity)
	if err != nil {
		return errors.Wrap(err)
	}

	existingRoleAssignmentsByScope := lo.GroupBy(existingRoleAssignments, func(roleAssignment armauthorization.RoleAssignment) string {
		return *roleAssignment.Properties.Scope
	})

	var expectedScopes []string
	for _, intent := range intents {
		scope, err := a.getIntentScope(intent)
		if err != nil {
			return errors.Wrap(err)
		}
		expectedScopes = append(expectedScopes, scope)

		roleNames := intent.AzureRoles
		existingRoleAssignmentsForScope := existingRoleAssignmentsByScope[scope]

		if err := a.ensureRoleAssignmentsForIntent(ctx, scope, roleNames, userAssignedIdentity, existingRoleAssignmentsForScope); err != nil {
			return errors.Wrap(err)
		}
	}

	if err := a.deleteRoleAssignmentsWithUnexpectedScopes(ctx, expectedScopes, existingRoleAssignments); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) ensureRoleAssignmentsForIntent(ctx context.Context, scope string, roleNames []string, userAssignedIdentity armmsi.Identity, existingRoleAssignmentsForScope []armauthorization.RoleAssignment) error {
	roleDefinitionsByName, err := a.FindRoleDefinitionByName(ctx, scope, roleNames)
	if err != nil {
		return errors.Wrap(err)
	}

	existingRoleDefinitionIDs := goset.FromSlice(lo.Map(existingRoleAssignmentsForScope, func(roleAssignment armauthorization.RoleAssignment, _ int) string {
		return *roleAssignment.Properties.RoleDefinitionID
	}))

	for _, roleName := range roleNames {
		roleDefinition := roleDefinitionsByName[roleName]
		roleDefinitionID := *roleDefinition.ID
		if !existingRoleDefinitionIDs.Contains(roleDefinitionID) {
			if err := a.CreateRoleAssignment(ctx, scope, userAssignedIdentity, roleDefinition); err != nil {
				return errors.Wrap(err)
			}
		}
	}

	expectedRoleDefinitionIDs := lo.Map(lo.Values(roleDefinitionsByName), func(roleDefinition armauthorization.RoleDefinition, _ int) string {
		return *roleDefinition.ID
	})
	if err := a.deleteRoleAssignmentsWithUnexpectedRoleDefID(ctx, expectedRoleDefinitionIDs, existingRoleAssignmentsForScope); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) deleteRoleAssignmentsWithUnexpectedRoleDefID(ctx context.Context, expectedRoleDefIDs []string, existingRoleAssignments []armauthorization.RoleAssignment) error {
	expectedRoleDefIDsSet := goset.FromSlice(expectedRoleDefIDs)
	for _, roleAssignment := range existingRoleAssignments {
		roleDefID := *roleAssignment.Properties.RoleDefinitionID
		if !expectedRoleDefIDsSet.Contains(roleDefID) {
			if err := a.DeleteRoleAssignment(ctx, roleAssignment); err != nil {
				return errors.Wrap(err)
			}
		}
	}

	return nil
}

func (a *Agent) deleteRoleAssignmentsWithUnexpectedScopes(ctx context.Context, expectedScopes []string, existingRoleAssignments []armauthorization.RoleAssignment) error {
	expectedScopesSet := goset.FromSlice(expectedScopes)
	for _, roleAssignment := range existingRoleAssignments {
		scope := *roleAssignment.Properties.Scope
		if !expectedScopesSet.Contains(scope) {
			if err := a.DeleteRoleAssignment(ctx, roleAssignment); err != nil {
				return errors.Wrap(err)
			}
		}
	}

	return nil
}

func (a *Agent) DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev1alpha3.ClientIntents) error {
	userAssignedIdentity, err := a.FindUserAssignedIdentity(ctx, intents.Namespace, intents.Spec.Service.Name)
	if err != nil {
		if errors.Is(err, azureagent.ErrUserIdentityNotFound) {
			return nil
		}
		return errors.Wrap(err)
	}

	existingRoleAssignments, err := a.ListRoleAssignments(ctx, userAssignedIdentity)
	if err != nil {
		return errors.Wrap(err)
	}

	for _, roleAssignment := range existingRoleAssignments {
		if err := a.DeleteRoleAssignment(ctx, roleAssignment); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}
