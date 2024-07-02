package azurepolicyagent

import (
	"context"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi"
	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"regexp"
	"strings"
)

var KeyVaultNameRegex = regexp.MustCompile(`^/subscriptions/[^/]+/resourceGroups/[^/]+/providers/Microsoft.KeyVault/vaults/([^/]+)$`)

type Agent struct {
	*azureagent.Agent
}

func NewAzurePolicyAgent(azureAgent *azureagent.Agent) *Agent {
	return &Agent{azureAgent}
}

func (a *Agent) IntentType() otterizev2alpha1.IntentType {
	return otterizev2alpha1.IntentTypeAzure
}

func (a *Agent) getIntentScope(intent otterizev2alpha1.Target) (string, error) {
	name := intent.GetTargetServerName()
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

func (a *Agent) AddRolePolicyFromIntents(ctx context.Context, namespace string, accountName string, intentsServiceName string, intents []otterizev2alpha1.Target, pod corev1.Pod) error {
	userAssignedIdentity, err := a.FindUserAssignedIdentity(ctx, namespace, intentsServiceName)
	if err != nil {
		return errors.Wrap(err)
	}

	azureRBACIntents := lo.Filter(intents, func(intent otterizev2alpha1.Target, _ int) bool {
		return intent.Azure != nil && len(intent.Azure.Roles) > 0
	})

	if err := a.ensureRoleAssignmentsForIntents(ctx, userAssignedIdentity, azureRBACIntents); err != nil {
		return errors.Wrap(err)
	}

	azureKeyVaultIntents := lo.Filter(intents, func(intent otterizev2alpha1.Target, _ int) bool {
		return intent.Azure != nil && intent.Azure.KeyVaultPolicy != nil
	})
	if err := a.ensureKeyVaultPermissionsForIntents(ctx, userAssignedIdentity, azureKeyVaultIntents); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) ensureRoleAssignmentsForIntents(ctx context.Context, userAssignedIdentity armmsi.Identity, intents []otterizev2alpha1.Target) error {
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

		roleNames := intent.Azure.Roles
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

func (a *Agent) DeleteRolePolicyFromIntents(ctx context.Context, intents otterizev2alpha1.ClientIntents) error {
	userAssignedIdentity, err := a.FindUserAssignedIdentity(ctx, intents.Namespace, intents.Spec.Workload.Name)
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

	existingKeyVaultsAccessPolicies, err := a.GetExistingKeyVaultAccessPolicies(ctx, userAssignedIdentity)
	if err != nil {
		return errors.Wrap(err)
	}

	for keyVaultName, _ := range existingKeyVaultsAccessPolicies {
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

func (a *Agent) ensureKeyVaultPermissionsForIntents(ctx context.Context, userAssignedIdentity armmsi.Identity, intents []otterizev2alpha1.Target) error {
	existingKeyVaultsAccessPolicies, err := a.GetExistingKeyVaultAccessPolicies(ctx, userAssignedIdentity)
	if err != nil {
		return errors.Wrap(err)
	}

	var expectedIntentsKeyVaults []string

	for _, intent := range intents {
		scope, err := a.getIntentScope(intent)
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
