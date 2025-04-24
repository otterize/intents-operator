package azurepolicyagent

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/agentutils"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"regexp"
	"strings"
	"sync"
)

const ReasonRoleDefinitionNotFound = "RoleDefinitionNotFound"

var KeyVaultNameRegex = regexp.MustCompile(`^/subscriptions/[^/]+/resourceGroups/[^/]+/providers/Microsoft.KeyVault/vaults/([^/]+)$`)
var StorageAccountRegex = regexp.MustCompile(`providers/Microsoft.Storage/storageAccounts/([^/]+)`)

type Agent struct {
	*azureagent.Agent
	roleMutex       sync.Mutex
	assignmentMutex sync.Mutex
	injectablerecorder.InjectableRecorder
}

func NewAzurePolicyAgent(ctx context.Context, azureAgent *azureagent.Agent) *Agent {
	agent := &Agent{Agent: azureAgent, roleMutex: sync.Mutex{}, assignmentMutex: sync.Mutex{}}
	return agent
}

func (a *Agent) IntentType() otterizev2alpha1.IntentType {
	return otterizev2alpha1.IntentTypeAzure
}

func (a *Agent) getIntentScope(ctx context.Context, intent otterizev2alpha1.Target) (string, error) {
	name := intent.GetTargetServerName()
	if !strings.HasPrefix(name, "/") {
		return "", errors.Errorf("expected intent name to start with /, got %s", name)
	}

	// If the scope is already a full scope, validate and return it
	if strings.HasPrefix(name, "/subscriptions/") {
		return name, nil
	}

	// If the scope is a partial storage account scope, find the full scope and return it
	if match := StorageAccountRegex.FindStringSubmatch(name); len(match) > 1 {
		storageAccountScope := fmt.Sprintf("/providers/Microsoft.Storage/storageAccounts/%s", match[1])
		fullScope, err := a.GetFullStorageResourceScope(ctx, storageAccountScope, name)
		if err != nil {
			return "", errors.Wrap(err)
		}
		return fullScope, nil
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

func (a *Agent) AddRolePolicyFromIntents(ctx context.Context, namespace string, _ string, intentsServiceName string, intents otterizev2alpha1.ClientIntents, _ []otterizev2alpha1.Target, _ corev1.Pod) error {
	userAssignedIdentity, exists, err := a.FindUserAssignedIdentity(ctx, namespace, intentsServiceName)
	if err != nil {
		return errors.Wrap(err)
	}
	if !exists {
		return errors.Errorf("%w: %s-%s", agentutils.ErrCloudIdentityNotFound, namespace, intentsServiceName)
	}

	if err := a.ensureCustomRolesForIntents(ctx, userAssignedIdentity, intents); err != nil {
		return errors.Wrap(err)
	}

	if err := a.ensureRoleAssignmentsForIntents(ctx, userAssignedIdentity, intents); err != nil {
		return errors.Wrap(err)
	}

	if err := a.ensureKeyVaultPermissionsForIntents(ctx, userAssignedIdentity, intents); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) ensureRoleAssignmentsForIntents(ctx context.Context, userAssignedIdentity armmsi.Identity, clientIntents otterizev2alpha1.ClientIntents) error {
	// Lock the agent to ensure that no other goroutine is modifying the assignments
	a.assignmentMutex.Lock()
	defer a.assignmentMutex.Unlock()

	existingRoleAssignments, err := a.ListRoleAssignmentsAcrossSubscriptions(ctx, &userAssignedIdentity)
	if err != nil {
		return errors.Wrap(err)
	}

	// Filter out assignments on built-in roles
	existingRoleAssignments = lo.Filter(existingRoleAssignments, func(roleAssignment armauthorization.RoleAssignment, _ int) bool {
		return !a.IsCustomRoleAssignment(roleAssignment)
	})

	existingRoleAssignmentsByScope := lo.GroupBy(existingRoleAssignments, func(roleAssignment armauthorization.RoleAssignment) string {
		return *roleAssignment.Properties.Scope
	})

	// Backwards compatibility for role assignments
	azureRBACIntents := lo.Filter(clientIntents.GetTargetList(), func(intent otterizev2alpha1.Target, _ int) bool {
		return intent.Azure != nil && len(intent.Azure.Roles) > 0
	})

	var expectedScopes []string
	for _, intent := range azureRBACIntents {
		scope, err := a.getIntentScope(ctx, intent)
		if err != nil {
			return errors.Wrap(err)
		}
		expectedScopes = append(expectedScopes, scope)

		roleNames := intent.Azure.Roles
		existingRoleAssignmentsForScope := existingRoleAssignmentsByScope[scope]

		if err := a.ensureRoleAssignmentsForIntent(ctx, scope, roleNames, userAssignedIdentity, existingRoleAssignmentsForScope, clientIntents); err != nil {
			return errors.Wrap(err)
		}
	}

	if err := a.deleteRoleAssignmentsWithUnexpectedScopes(ctx, expectedScopes, existingRoleAssignments); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) ensureRoleAssignmentsForIntent(
	ctx context.Context,
	scope string,
	roleNames []string,
	userAssignedIdentity armmsi.Identity,
	existingRoleAssignmentsForScope []armauthorization.RoleAssignment,
	intents otterizev2alpha1.ClientIntents,
) error {
	roleDefinitionsByName, err := a.FindRoleDefinitionByName(ctx, scope, roleNames)
	if err != nil {
		return errors.Wrap(err)
	}
	existingRoleDefinitionIDs := goset.FromSlice(lo.Map(existingRoleAssignmentsForScope, func(roleAssignment armauthorization.RoleAssignment, _ int) string {
		return *roleAssignment.Properties.RoleDefinitionID
	}))

	for _, roleName := range roleNames {
		roleDefinition, exists := roleDefinitionsByName[roleName]
		if !exists {
			// The role mentioned in the intents does not exist in the subscription, record event and move on
			a.RecordWarningEventf(&intents, ReasonRoleNotFound, "Role %s not found in scope %s", roleName, scope)
			continue
		}
		roleDefinitionID := *roleDefinition.ID
		if !existingRoleDefinitionIDs.Contains(roleDefinitionID) {
			if err := a.CreateRoleAssignment(ctx, scope, userAssignedIdentity, roleDefinition, nil); err != nil {
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

func (a *Agent) DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev2alpha1.ClientIntents) error {
	accountName := intents.Spec.Workload.Name
	userAssignedIdentity, exists, err := a.FindUserAssignedIdentity(ctx, intents.Namespace, accountName)
	if err != nil {
		return errors.Wrap(err)
	}
	if !exists {
		return errors.Errorf("%w: %s-%s", agentutils.ErrCloudIdentityNotFound, intents.Namespace, accountName)
	}

	existingRoleAssignments, err := a.ListRoleAssignmentsAcrossSubscriptions(ctx, &userAssignedIdentity)
	if err != nil {
		return errors.Wrap(err)
	}

	for _, roleAssignment := range existingRoleAssignments {
		if err := a.DeleteRoleAssignment(ctx, roleAssignment); err != nil {
			return errors.Wrap(err)
		}
	}

	// Custom roles
	azureCustomRolesTargets := lo.Filter(intents.GetTargetList(), func(intent otterizev2alpha1.Target, _ int) bool {
		hasCustomRoles := intent.Azure != nil && (len(intent.Azure.Actions) > 0 || len(intent.Azure.DataActions) > 0)
		return hasCustomRoles && len(intent.Azure.Roles) == 0
	})

	for _, target := range azureCustomRolesTargets {
		scope, err := a.getIntentScope(ctx, target)
		if err != nil {
			return errors.Wrap(err)
		}

		err = a.DeleteCustomRole(ctx, scope, intents)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	existingKeyVaultsAccessPolicies, err := a.GetExistingKeyVaultAccessPolicies(ctx, userAssignedIdentity)
	if err != nil {
		return errors.Wrap(err)
	}

	for keyVaultName := range existingKeyVaultsAccessPolicies {
		if err := a.RemoveKeyVaultAccessPolicy(ctx, keyVaultName, userAssignedIdentity); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func extractKeyVaultName(scope string) (string, error) {
	match := KeyVaultNameRegex.FindStringSubmatch(scope)
	if len(match) != 2 {
		return "", errors.Errorf("expected intent scope to match key vault name regex, got %s", scope)
	}

	return match[1], nil
}

func (a *Agent) ensureKeyVaultPermissionsForIntents(ctx context.Context, userAssignedIdentity armmsi.Identity, clientIntents otterizev2alpha1.ClientIntents) error {
	existingKeyVaultsAccessPolicies, err := a.GetExistingKeyVaultAccessPolicies(ctx, userAssignedIdentity)
	if err != nil {
		return errors.Wrap(err)
	}

	var expectedIntentsKeyVaults []string

	// Key Vault permissions
	azureKeyVaultIntents := lo.Filter(clientIntents.GetTargetList(), func(intent otterizev2alpha1.Target, _ int) bool {
		return intent.Azure != nil && intent.Azure.KeyVaultPolicy != nil
	})

	for _, intent := range azureKeyVaultIntents {
		scope, err := a.getIntentScope(ctx, intent)
		if err != nil {
			return errors.Wrap(err)
		}

		keyVaultName, err := extractKeyVaultName(scope)
		if err != nil {
			return errors.Wrap(err)
		}

		expectedIntentsKeyVaults = append(expectedIntentsKeyVaults, keyVaultName)

		if err := a.ensureKeyVaultPolicyForIntent(ctx, userAssignedIdentity, keyVaultName, intent, existingKeyVaultsAccessPolicies); err != nil {
			return errors.Wrap(err)
		}
	}

	if err := a.removeUnexpectedKeyVaultPolicies(ctx, userAssignedIdentity, expectedIntentsKeyVaults, existingKeyVaultsAccessPolicies); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) ensureKeyVaultPolicyForIntent(ctx context.Context, userAssignedIdentity armmsi.Identity, keyVaultName string, intent otterizev2alpha1.Target, existingKeyVaultsAccessPolicies map[string][]*armkeyvault.AccessPolicyEntry) error {
	existingAccessPolicies, ok := existingKeyVaultsAccessPolicies[keyVaultName]
	if !ok {
		return errors.Errorf("key vault %s not found", keyVaultName)
	}

	policy := a.vaultAccessPolicyEntryFromIntent(userAssignedIdentity, lo.FromPtr(intent.Azure.KeyVaultPolicy))

	if len(existingAccessPolicies) == 0 {
		// add new policy
		if err := a.AddKeyVaultAccessPolicy(ctx, keyVaultName, policy); err != nil {
			return errors.Wrap(err)
		}
		return nil
	}

	if !azureagent.AccessPoliciesEqual(existingAccessPolicies[0], &policy) {
		// update existing policy
		if err := a.ReplaceKeyVaultAccessPolicy(ctx, keyVaultName, policy); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (a *Agent) removeUnexpectedKeyVaultPolicies(ctx context.Context, userAssignedIdentity armmsi.Identity, expectedKeyVaultNames []string, existingKeyVaultsAccessPolicies map[string][]*armkeyvault.AccessPolicyEntry) error {
	unexpectedKeyVaultNames := lo.Without(lo.Keys(existingKeyVaultsAccessPolicies), expectedKeyVaultNames...)

	for _, keyVaultName := range unexpectedKeyVaultNames {
		if err := a.RemoveKeyVaultAccessPolicy(ctx, keyVaultName, userAssignedIdentity); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (a *Agent) vaultAccessPolicyEntryFromIntent(userAssignedIdentity armmsi.Identity, policy otterizev2alpha1.AzureKeyVaultPolicy) armkeyvault.AccessPolicyEntry {
	return armkeyvault.AccessPolicyEntry{
		ObjectID: userAssignedIdentity.Properties.ClientID,
		TenantID: &a.Conf.TenantID,
		Permissions: &armkeyvault.Permissions{
			Certificates: lo.Map(policy.CertificatePermissions, func(p otterizev2alpha1.AzureKeyVaultCertificatePermission, _ int) *armkeyvault.CertificatePermissions {
				return lo.ToPtr(armkeyvault.CertificatePermissions(p))
			}),
			Keys: lo.Map(policy.KeyPermissions, func(p otterizev2alpha1.AzureKeyVaultKeyPermission, _ int) *armkeyvault.KeyPermissions {
				return lo.ToPtr(armkeyvault.KeyPermissions(p))
			}),
			Secrets: lo.Map(policy.SecretPermissions, func(p otterizev2alpha1.AzureKeyVaultSecretPermission, _ int) *armkeyvault.SecretPermissions {
				return lo.ToPtr(armkeyvault.SecretPermissions(p))
			}),
			Storage: lo.Map(policy.StoragePermissions, func(p otterizev2alpha1.AzureKeyVaultStoragePermission, _ int) *armkeyvault.StoragePermissions {
				return lo.ToPtr(armkeyvault.StoragePermissions(p))
			}),
		},
	}
}

func (a *Agent) ensureCustomRolesForIntents(ctx context.Context, userAssignedIdentity armmsi.Identity, clientIntents otterizev2alpha1.ClientIntents) error {
	// Lock the agent to ensure that no other goroutine is modifying the custom roles in parallel
	a.roleMutex.Lock()
	defer a.roleMutex.Unlock()

	existingRoleAssignments, err := a.ListRoleAssignmentsAcrossSubscriptions(ctx, &userAssignedIdentity)
	if err != nil {
		return errors.Wrap(err)
	}

	// Filter out non-otterize roles
	existingRoleAssignments = lo.Filter(existingRoleAssignments, func(roleAssignment armauthorization.RoleAssignment, _ int) bool {
		return a.IsCustomRoleAssignment(roleAssignment)
	})

	// Custom roles
	azureCustomRolesTargets := lo.Filter(clientIntents.GetTargetList(), func(intent otterizev2alpha1.Target, _ int) bool {
		hasCustomRoles := intent.Azure != nil && (len(intent.Azure.Actions) > 0 || len(intent.Azure.DataActions) > 0)
		return hasCustomRoles && len(intent.Azure.Roles) == 0
	})

	var expectedScopes []string
	for _, target := range azureCustomRolesTargets {
		scope, err := a.getIntentScope(ctx, target)
		if err != nil {
			return errors.Wrap(err)
		}

		expectedScopes = append(expectedScopes, scope)

		err = a.ensureCustomRoleForIntent(ctx, userAssignedIdentity, scope, clientIntents, target)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	if err := a.deleteRoleAssignmentsWithUnexpectedScopes(ctx, expectedScopes, existingRoleAssignments); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) ensureCustomRoleForIntent(ctx context.Context, userAssignedIdentity armmsi.Identity, scope string, intents otterizev2alpha1.ClientIntents, intent otterizev2alpha1.Target) error {
	actions := intent.Azure.Actions
	dataActions := intent.Azure.DataActions

	customRoleName := a.GenerateCustomRoleName(intents, scope)
	role, found := a.FindCustomRoleByName(ctx, scope, customRoleName)
	if found {
		err := a.UpdateCustomRole(ctx, scope, role, actions, dataActions)
		if err != nil {
			return errors.Wrap(err)
		}
	} else {
		if err := a.ValidateScope(ctx, scope); err != nil {
			// Prevent using non-existing scopes for custom roles,
			// as they may cause issues when deleting the custom role
			return errors.Wrap(err)
		}

		newRole, err := a.CreateCustomRole(ctx, scope, intents, actions, dataActions)
		if err != nil {
			return errors.Wrap(err)
		}

		// create a role assignment for the custom role
		err = a.CreateRoleAssignment(ctx, scope, userAssignedIdentity, *newRole, to.Ptr(azureagent.OtterizeRoleAssignmentTag))
		if err != nil {
			// TODO: handle case when custom role is created and role assignment fails
			return errors.Wrap(err)
		}
	}

	return nil
}
