package azurepolicyagent

import (
	"context"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"strings"
)

func (a *Agent) PeriodicTasks(ctx context.Context) {
	err := a.CleanupCustomRoles(ctx)
	if err != nil {
		logrus.WithError(err).Error("Failed to clean up custom roles")
	}
}

// CleanupCustomRoles deletes custom roles that got orphaned due to the deletion of its role assignments
// when deleting a role assignment, the custom role might not get deleted if some error occurs (DeleteRoleAssignment)
func (a *Agent) CleanupCustomRoles(ctx context.Context) error {
	// Lock the agent to ensure that no other goroutine is modifying the custom roles in parallel
	a.roleMutex.Lock()
	defer a.roleMutex.Unlock()

	// List otterize custom roles across all subscriptions
	existingCustomRoles, err := a.ListCustomRolesAcrossSubscriptions(ctx)
	if err != nil {
		return errors.Wrap(err)
	}

	// Mark all custom roles as not in use
	rolesInUse := make(map[string]bool)
	for _, role := range existingCustomRoles {
		rolesInUse[*role.ID] = false
	}

	// List otterize role assignments across all subscriptions
	existingRoleAssignments, err := a.ListRoleAssignmentsAcrossSubscriptions(ctx, nil)
	if err != nil {
		return errors.Wrap(err)
	}

	customRoleAssignments := lo.Filter(existingRoleAssignments, func(roleAssignment armauthorization.RoleAssignment, _ int) bool {
		return a.IsCustomRoleAssignment(roleAssignment)
	})

	// Mark custom roles that are in use
	for _, roleAssignment := range customRoleAssignments {
		roleID := *roleAssignment.Properties.RoleDefinitionID

		if _, ok := rolesInUse[roleID]; !ok {
			// This otterize role assignment is linked to a non-otterize custom role - should not happen in production
			// If this does happen in testing environments we clean up the role assignment and the custom role it's linked to
			err := a.DeleteRoleAssignment(ctx, roleAssignment)

			logrus.WithError(err).
				WithField("role", roleID).
				WithField("assignment", roleAssignment.Name).
				Error("Otterize custom role assignment to a non-otterize custom role found")
			continue
		}

		rolesInUse[roleID] = true
	}

	// Delete custom roles that are not in use
	for roleID, inUse := range rolesInUse {
		if !inUse {
			roleScope := a.GetSubscriptionScope(roleID)
			roleDefinitionID := roleID[strings.LastIndex(roleID, "/")+1:]

			logrus.Warnf("Deleting custom role %s", roleID)
			err := a.DeleteCustomRole(ctx, roleScope, roleDefinitionID)
			if err != nil {
				return errors.Wrap(err)
			}
		}
	}

	return nil
}
