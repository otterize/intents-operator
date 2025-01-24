package azureagent

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	"strings"
)

func (a *Agent) PeriodicTasks() {
	ctx := context.TODO()

	err := a.CleanupCustomRoles(ctx)
	if err != nil {
		logrus.WithError(err).Error("Failed to clean up custom roles")
	}
}

// CleanupCustomRoles deletes custom roles that got orphaned due to the deletion of its role assignments
// when deleting a role assignment, the custom role might not get deleted if some error occurs (DeleteRoleAssignment)
func (a *Agent) CleanupCustomRoles(ctx context.Context) error {
	// List otterize custom roles across all subscriptions
	existingCustomRoles, err := a.ListCustomRolesAcrossSubscriptions(ctx)
	if err != nil {
		fmt.Println("Error: ", err)
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

	// Mark custom roles that are in use
	for _, roleAssignment := range existingRoleAssignments {
		if a.IsCustomRoleAssignment(roleAssignment) {
			roleID := *roleAssignment.Properties.RoleDefinitionID

			if _, ok := rolesInUse[roleID]; !ok {
				// This otterize role assignment is linked to a non-otterize custom role - should not happen
				err := a.DeleteRoleAssignment(ctx, roleAssignment)

				logrus.WithError(err).
					WithField("role", roleID).
					WithField("assignment", roleAssignment.Name).
					Error("Otterize custom role assignment to a non-otterize custom role found")
				continue
			}

			rolesInUse[roleID] = true
		}
	}

	// Delete custom roles that are not in use
	for roleID, inUse := range rolesInUse {
		if !inUse {
			roleScope := a.getSubscriptionScope(roleID)
			roleDefinitionID := roleID[strings.LastIndex(roleID, "/")+1:]

			err := a.DeleteCustomRole(ctx, roleScope, roleDefinitionID)
			if err != nil {
				return errors.Wrap(err)
			}
		}
	}

	return nil
}
