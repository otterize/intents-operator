package awsagent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/rolesanywhere"
	rolesanywhereTypes "github.com/aws/aws-sdk-go-v2/service/rolesanywhere/types"
	"github.com/aws/smithy-go"
	"github.com/otterize/intents-operator/src/shared/agentutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
	"net/url"
	"strings"
	"sync"
	"time"
)
import "github.com/aws/aws-sdk-go-v2/service/iam"

// GetOtterizeRole gets the matching IAM role for given service
// Return value is:
// - found (bool), true if role exists, false if not
// - role (types.Role), aws role
// - error
func (a *Agent) GetOtterizeRole(ctx context.Context, namespaceName, accountName string) (bool, *types.Role, error) {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", accountName)
	roleName := a.generateRoleName(namespaceName, accountName)

	role, err := a.iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})

	if err != nil {
		var apiError smithy.APIError
		if errors.As(err, &apiError) {
			var noSuchEntityException *types.NoSuchEntityException
			switch {
			case errors.As(apiError, &noSuchEntityException):
				logger.WithError(err).Debug("role not found")
				return false, nil, nil
			default:
				return false, nil, errors.Wrap(err)
			}
		}

		return false, nil, errors.Wrap(err)
	}

	return true, role.Role, nil
}

func (a *Agent) loadNextProfileCachePage(ctx context.Context, nextToken *string) (*string, error) {
	output, err := a.rolesAnywhereClient.ListProfiles(ctx, &rolesanywhere.ListProfilesInput{NextToken: nextToken})
	if err != nil {
		return nil, errors.Errorf("failed to list profiles: %w", err)
	}

	for _, profile := range output.Profiles {
		a.profileNameToId[*profile.Name] = *profile.ProfileId
	}
	return output.NextToken, nil
}

func (a *Agent) initProfileCache(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			a.profileNameToId = make(map[string]string)
			a.profileCacheOnce = sync.Once{}
		}
	}()
	a.profileCacheOnce.Do(func() {
		var nextToken *string
		nextToken, err = a.loadNextProfileCachePage(ctx, nil)
		if err != nil {
			return
		}

		for nextToken != nil {
			nextToken, err = a.loadNextProfileCachePage(ctx, nextToken)
			if err != nil {
				return
			}
		}
	})
	return errors.Wrap(err)
}

func (a *Agent) GetOtterizeProfile(ctx context.Context, namespaceName, serviceAccountName string) (found bool, profile *rolesanywhereTypes.ProfileDetail, err error) {
	err = a.initProfileCache(ctx)
	if err != nil {
		return false, nil, errors.Errorf("failed to initialize profile cache: %w", err)
	}

	if profileId, ok := a.profileNameToId[a.generateRolesAnywhereProfileName(namespaceName, serviceAccountName)]; ok {
		getProfileOutput, err := a.rolesAnywhereClient.GetProfile(ctx, &rolesanywhere.GetProfileInput{ProfileId: &profileId})
		if err != nil {
			if noSuchEntity := (types.NoSuchEntityException{}); errors.As(err, &noSuchEntity) {
				delete(a.profileNameToId, a.generateRolesAnywhereProfileName(namespaceName, serviceAccountName))
				return false, nil, nil
			}
			return false, nil, errors.Errorf("failed to get profile: %w", err)
		}

		return true, getProfileOutput.Profile, nil
	}

	return false, nil, nil
}

func (a *Agent) DeleteRolesAnywhereProfileForServiceAccount(ctx context.Context, namespace string, serviceAccountName string) (bool, error) {
	logger := logrus.WithField("namespace", namespace).WithField("serviceAccount", serviceAccountName)

	found, profile, err := a.GetOtterizeProfile(ctx, namespace, serviceAccountName)
	if err != nil {
		return false, errors.Wrap(err)
	}

	if !found {
		return false, nil
	}

	logger.WithField("profile", *profile.Name).Info("deleting rolesanywhere profile")

	_, err = a.rolesAnywhereClient.DeleteProfile(ctx, &rolesanywhere.DeleteProfileInput{ProfileId: profile.ProfileId})
	if err != nil {
		return false, errors.Wrap(err)
	}

	delete(a.profileNameToId, *profile.Name)

	return true, nil
}

func (a *Agent) CreateRolesAnywhereProfileForRole(ctx context.Context, role types.Role, namespace string, serviceAccountName string) (profile *rolesanywhereTypes.ProfileDetail, err error) {
	found, profile, err := a.GetOtterizeProfile(ctx, namespace, serviceAccountName)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	if found {
		return profile, nil
	}

	createProfileOutput, createProfileErr := a.rolesAnywhereClient.CreateProfile(ctx,
		&rolesanywhere.CreateProfileInput{
			RoleArns: []string{*role.Arn},
			Enabled:  lo.ToPtr(true),
			Name:     lo.ToPtr(a.generateRolesAnywhereProfileName(namespace, serviceAccountName)),
			Tags: []rolesanywhereTypes.Tag{
				{
					Key:   aws.String(serviceAccountNameTagKey),
					Value: aws.String(serviceAccountName),
				},
				{
					Key:   aws.String(serviceAccountNamespaceTagKey),
					Value: aws.String(namespace),
				},
				{
					Key:   aws.String(clusterNameTagKey),
					Value: aws.String(a.ClusterName),
				},
			},
		})

	if createProfileErr != nil {
		return nil, errors.Wrap(createProfileErr)
	}

	a.profileNameToId[*createProfileOutput.Profile.Name] = *createProfileOutput.Profile.ProfileId

	return createProfileOutput.Profile, nil
}

func compareStatements(existingStatement StatementEntry, newStatement StatementEntry) bool {
	// Fields: Effect, Action, Resource, Principal, Sid, Condition
	// Action is a slice
	// Principal is a map
	// Condition is a map
	if newStatement.Effect != existingStatement.Effect {
		return false
	}
	slices.Sort(existingStatement.Action)
	slices.Sort(newStatement.Action)
	if !slices.Equal(existingStatement.Action, newStatement.Action) {
		return false
	}
	if newStatement.Resource != existingStatement.Resource {
		return false
	}
	existingPrincipals := lo.MapToSlice(existingStatement.Principal, func(key string, value string) string {
		return fmt.Sprintf("%s:%s", key, value)
	})
	newPrincipals := lo.MapToSlice(newStatement.Principal, func(key string, value string) string {
		return fmt.Sprintf("%s:%s", key, value)
	})
	slices.Sort(existingPrincipals)
	slices.Sort(newPrincipals)
	if !slices.Equal(existingPrincipals, newPrincipals) {
		return false
	}
	if existingStatement.Sid != newStatement.Sid {
		return false
	}
	if len(existingStatement.Condition) != len(newStatement.Condition) {
		return false
	}
	existingCondition := lo.MapToSlice(existingStatement.Condition, func(key string, value any) string {
		return fmt.Sprintf("%s:%s", key, value)
	})
	newCondition := lo.MapToSlice(newStatement.Condition, func(key string, value any) string {
		return fmt.Sprintf("%s:%s", key, value)
	})
	slices.Sort(existingCondition)
	slices.Sort(newCondition)
	if !slices.Equal(existingCondition, newCondition) {
		return false
	}
	return true
}

func (a *Agent) IsPolicyDocumentsIdenticalUnordered(role *types.Role, newDocument PolicyDocument) (bool, error) {
	if role.AssumeRolePolicyDocument == nil {
		return false, errors.New("role AssumeRolePolicyDocument is nil")
	}

	// AssumeRolePolicyDocument is url-encoded
	assumeRolePolicyDocumentRaw, err := url.QueryUnescape(*role.AssumeRolePolicyDocument)
	if err != nil {
		return false, errors.Wrap(err)
	}

	var existingPolicy map[string]any
	err = json.Unmarshal([]byte(assumeRolePolicyDocumentRaw), &existingPolicy)
	if err != nil {
		return false, errors.Wrap(err)
	}

	// For some reason, Action can either be a list or a string, so it can't be unmarshalled into the same struct.
	// Let's make sure it's always a list.
	statements := existingPolicy["Statement"].([]any)
	for i, statement := range statements {
		statementTyped := statement.(map[string]any)
		// Get action or actions
		if action, ok := statementTyped["Action"]; ok {
			switch actionType := action.(type) {
			case string:
				statementTyped["Action"] = []string{actionType} // Convert single string to slice
				statements[i] = statementTyped                  // Update the statement in the slice
			case []any:
				// do nothing
			default:
				return false, errors.Errorf("unexpected type for Action: %T", actionType)
			}
		}
	}
	existingPolicy["Statement"] = statements

	// Marshal the policy back to JSON
	assumeRolePolicyDocument, err := json.Marshal(existingPolicy)
	if err != nil {
		return false, errors.Wrap(err)
	}

	var existingPolicyStructured PolicyDocument
	err = json.Unmarshal(assumeRolePolicyDocument, &existingPolicyStructured)
	if err != nil {
		return false, errors.Wrap(err)
	}

	if existingPolicyStructured.Version != newDocument.Version {
		return false, nil
	}

	if len(existingPolicyStructured.Statement) != len(newDocument.Statement) {
		return false, nil
	}

	for _, newStatement := range newDocument.Statement {
		_, found := lo.Find(existingPolicyStructured.Statement, func(existingStatement StatementEntry) bool {
			return compareStatements(existingStatement, newStatement)
		})

		if !found {
			return false, nil
		}
	}

	return true, nil
}

// CreateOtterizeIAMRole creates a new IAM role for service, if one doesn't exist yet
func (a *Agent) CreateOtterizeIAMRole(ctx context.Context, namespaceName string, accountName string, useSoftDeleteStrategy bool, additionalTrustRelationshipEntries []StatementEntry) (*types.Role, error) {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", accountName)
	exists, role, err := a.GetOtterizeRole(ctx, namespaceName, accountName)

	if err != nil {
		return nil, errors.Wrap(err)
	}

	trustPolicy := a.generateCompleteTrustPolicyAnyIdentityProvider(logger, namespaceName, accountName, additionalTrustRelationshipEntries)

	identicalPolicyDocuments := false
	if exists {
		identicalPolicyDocuments, err = a.IsPolicyDocumentsIdenticalUnordered(role, trustPolicy)
		if err != nil {
			return nil, errors.Wrap(err)
		}
	}

	serializedPolicyDocument, err := json.Marshal(trustPolicy)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	if exists {
		if identicalPolicyDocuments {
			// Role exists and everything is identical - we handle soft delete if needed or just return it if not
			return a.handleSoftDeleteOrReturnUnchanged(ctx, logger, role, useSoftDeleteStrategy)
		}
		logger.WithField("role", *role.RoleName).Info("updating existing role with new trust policy")
		_, err = a.iamClient.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
			RoleName:       role.RoleName,
			PolicyDocument: aws.String(string(serializedPolicyDocument)),
		})
		role.AssumeRolePolicyDocument = lo.ToPtr(string(serializedPolicyDocument))
		if err != nil {
			return nil, errors.Wrap(err)
		}
		logger.WithField("arn", *role.Arn).Debug("updated existing role")
		return role, nil
	}

	tags := []types.Tag{
		{
			Key:   aws.String(serviceAccountNameTagKey),
			Value: aws.String(accountName),
		},
		{
			Key:   aws.String(serviceAccountNamespaceTagKey),
			Value: aws.String(namespaceName),
		},
		{
			Key:   aws.String(clusterNameTagKey),
			Value: aws.String(a.ClusterName),
		},
	}
	if useSoftDeleteStrategy {
		tags = append(tags, types.Tag{Key: aws.String(softDeletionStrategyTagKey), Value: aws.String(softDeletionStrategyTagValue)})
	}
	createRoleInput := &iam.CreateRoleInput{
		RoleName:                 aws.String(a.generateRoleName(namespaceName, accountName)),
		AssumeRolePolicyDocument: aws.String(string(serializedPolicyDocument)),
		Tags:                     tags,
		Description:              aws.String(iamRoleDescription),
		PermissionsBoundary:      aws.String(fmt.Sprintf("arn:aws:iam::%s:policy/%s-limit-iam-permission-boundary", a.AccountID, a.ClusterName)),
	}
	createRoleOutput, createRoleError := a.iamClient.CreateRole(ctx, createRoleInput)

	if createRoleError != nil {
		return nil, errors.Wrap(createRoleError)
	}

	logger.Debugf("created new role, arn: %s", *createRoleOutput.Role.Arn)
	return createRoleOutput.Role, nil

}

func (a *Agent) handleSoftDeleteOrReturnUnchanged(ctx context.Context, logger *logrus.Entry, role *types.Role, useSoftDeleteStrategy bool) (*types.Role, error) {
	logger.WithField("arn", *role.Arn).Debug("found existing role")
	// check if it is soft deleted - if so remove soft deleted tag
	if hasSoftDeletedTagSet(role.Tags) {
		logger.Debug("role is tagged unused, untagging")
		// There is no need to untag the role's policies from this context, It will happen if the policy will be created again
		_, err := a.iamClient.UntagRole(ctx, &iam.UntagRoleInput{RoleName: role.RoleName, TagKeys: []string{softDeletedTagKey}})
		if err != nil {
			return nil, errors.Wrap(err)
		}
		// Intentionally not returning here, as we want to continue and check if we need to mark as soft delete only
	}
	if useSoftDeleteStrategy {
		err := a.setRoleSoftDeleteStrategyTag(ctx, role)
		if err != nil {
			return nil, errors.Wrap(err)
		}
	}
	if !useSoftDeleteStrategy && HasSoftDeleteStrategyTagSet(role.Tags) {
		err := a.UnsetRoleSoftDeleteStrategyTag(ctx, role)
		if err != nil {
			return nil, errors.Wrap(err)
		}
	}

	return role, nil
}

func (a *Agent) DeleteOtterizeIAMRole(ctx context.Context, namespaceName, accountName string) error {
	logger := logrus.WithField("namespace", namespaceName).WithField("account", accountName)
	exists, role, err := a.GetOtterizeRole(ctx, namespaceName, accountName)
	if err != nil {
		return errors.Wrap(err)
	}

	if !exists {
		logger.Info("role does not exist, not doing anything")
		return nil
	}

	tags := lo.SliceToMap(role.Tags, func(item types.Tag) (string, string) {
		return *item.Key, *item.Value
	})

	if taggedServiceAccountName, ok := tags[serviceAccountNameTagKey]; !ok || taggedServiceAccountName != accountName {
		return errors.Errorf("attempted to delete role with incorrect service account name: expected '%s' but got '%s'", accountName, taggedServiceAccountName)
	}
	if taggedNamespace, ok := tags[serviceAccountNamespaceTagKey]; !ok || taggedNamespace != namespaceName {
		return errors.Errorf("attempted to delete role with incorrect namespace: expected '%s' but got '%s'", namespaceName, taggedNamespace)
	}
	if taggedClusterName, ok := tags[clusterNameTagKey]; !ok || taggedClusterName != a.ClusterName {
		return errors.Errorf("attempted to delete role with incorrect cluster name: expected '%s' but got '%s'", a.ClusterName, taggedClusterName)
	}

	if HasSoftDeleteStrategyTagSet(role.Tags) {
		logger.Info("role has softDeleteStrategy tag, tagging as unused")
		return a.SoftDeleteOtterizeIAMRole(ctx, role)
	}

	err = a.deleteAllRolePolicies(ctx, role)

	if err != nil {
		return errors.Wrap(err)
	}

	logger.WithField("role", *role.RoleName).Info("deleting IAM role")
	_, err = a.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: role.RoleName,
	})

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (a *Agent) SoftDeleteOtterizeIAMRole(ctx context.Context, role *types.Role) error {
	err := a.SoftDeleteAllRolePolicies(ctx, role)
	if err != nil {
		return errors.Wrap(err)
	}

	_, err = a.iamClient.TagRole(ctx, &iam.TagRoleInput{
		RoleName: role.RoleName,
		Tags:     []types.Tag{{Key: aws.String(softDeletedTagKey), Value: aws.String(time.Now().String())}},
	})

	if err != nil {
		return errors.Wrap(err)
	}

	return nil
}

// deleteAllRolePolicies deletes the inline role policy, if exists, in preparation to delete the role.
func (a *Agent) deleteAllRolePolicies(ctx context.Context, role *types.Role) error {
	listOutput, err := a.iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: role.RoleName})

	if err != nil {
		return errors.Errorf("failed to list role attached policies: %w", err)
	}

	for _, policy := range listOutput.AttachedPolicies {
		if strings.HasPrefix(*policy.PolicyName, "otterize-") || strings.HasPrefix(*policy.PolicyName, "otr-") {
			err = a.DeleteRolePolicy(ctx, *policy.PolicyName)
			if err != nil {
				return errors.Errorf("failed to delete policy: %w", err)
			}
		}
	}

	return nil
}

// SoftDeleteAllRolePolicies marks all inline role policies as deleted.
func (a *Agent) SoftDeleteAllRolePolicies(ctx context.Context, role *types.Role) error {
	listOutput, err := a.iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: role.RoleName})

	if err != nil {
		return errors.Errorf("failed to list role attached policies: %w", err)
	}

	for _, policy := range listOutput.AttachedPolicies {
		if strings.HasPrefix(*policy.PolicyName, "otterize-") || strings.HasPrefix(*policy.PolicyName, "otr-") {
			err = a.softDeletePolicy(ctx, *policy.PolicyName)
			if err != nil {
				return errors.Errorf("failed to delete policy: %w", err)
			}
		}
	}

	return nil
}

// setRoleSoftDeleteStrategyTag sets the soft delete strategy tag on the role
func (a *Agent) setRoleSoftDeleteStrategyTag(ctx context.Context, role *types.Role) error {
	err := a.setAllRolePoliciesSoftDeleteStrategyTag(ctx, role)
	if err != nil {
		return errors.Wrap(err)
	}
	_, err = a.iamClient.TagRole(ctx, &iam.TagRoleInput{RoleName: role.RoleName, Tags: []types.Tag{{Key: aws.String(softDeletionStrategyTagKey), Value: aws.String(softDeletionStrategyTagValue)}}})
	return errors.Wrap(err)
}

// setAllRolePoliciesSoftDeleteStrategyTag sets the soft delete strategy tag on all inline role policies
func (a *Agent) setAllRolePoliciesSoftDeleteStrategyTag(ctx context.Context, role *types.Role) error {
	listOutput, err := a.iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: role.RoleName})

	if err != nil {
		return errors.Errorf("failed to list role attached policies: %w", err)
	}

	for _, policy := range listOutput.AttachedPolicies {
		if strings.HasPrefix(*policy.PolicyName, "otterize-") || strings.HasPrefix(*policy.PolicyName, "otr-") {
			_, err = a.iamClient.TagPolicy(ctx, &iam.TagPolicyInput{PolicyArn: policy.PolicyArn, Tags: []types.Tag{{Key: aws.String(softDeletionStrategyTagKey), Value: aws.String(softDeletionStrategyTagValue)}}})
			if err != nil {
				return errors.Errorf("failed to set policy soft delete strategy tag: %w", err)
			}
		}
	}

	return nil
}

// UnsetRoleSoftDeleteStrategyTag removes the soft delete strategy tag from the role
func (a *Agent) UnsetRoleSoftDeleteStrategyTag(ctx context.Context, role *types.Role) error {
	err := a.unsetAllRolePoliciesSoftDeleteStrategyTag(ctx, role)
	if err != nil {
		return errors.Wrap(err)
	}
	_, err = a.iamClient.UntagRole(ctx, &iam.UntagRoleInput{RoleName: role.RoleName, TagKeys: []string{softDeletionStrategyTagKey}})
	return errors.Wrap(err)
}

// unsetAllRolePoliciesSoftDeleteStrategyTag removes the soft delete strategy tag from all inline role policies
func (a *Agent) unsetAllRolePoliciesSoftDeleteStrategyTag(ctx context.Context, role *types.Role) error {
	listOutput, err := a.iamClient.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{RoleName: role.RoleName})

	if err != nil {
		return errors.Errorf("failed to list role attached policies: %w", err)
	}

	for _, policy := range listOutput.AttachedPolicies {
		if strings.HasPrefix(*policy.PolicyName, "otterize-") || strings.HasPrefix(*policy.PolicyName, "otr-") {
			_, err = a.iamClient.UntagPolicy(ctx, &iam.UntagPolicyInput{PolicyArn: policy.PolicyArn, TagKeys: []string{softDeletionStrategyTagKey}})
			if err != nil {
				return errors.Errorf("failed to unset policy soft delete strategy tag: %w", err)
			}
		}
	}

	return nil
}

func (a *Agent) generateCompleteTrustPolicyAnyIdentityProvider(logger *logrus.Entry, namespaceName string, accountName string, additionalEntries []StatementEntry) PolicyDocument {
	policy := a.generateBaseTrustPolicyOIDC(namespaceName, accountName)

	if a.RolesAnywhereEnabled {
		policy = a.generateBaseTrustPolicyForRolesAnywhere(namespaceName, accountName)
	}

	if len(additionalEntries) > 0 {
		logger.WithField("statements", additionalEntries).Debug("Adding additional trust relationship statements to role")
		policy.Statement = append(policy.Statement, additionalEntries...)
	}

	return policy
}

func (a *Agent) generateBaseTrustPolicyOIDC(namespaceName string, accountName string) PolicyDocument {
	oidc := strings.TrimPrefix(a.OidcURL, "https://")

	policy := PolicyDocument{
		Version: iamAPIVersion,
		Statement: []StatementEntry{
			{
				Effect: iamEffectAllow,
				Action: []string{"sts:AssumeRoleWithWebIdentity"},
				Principal: map[string]string{
					"Federated": fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", a.AccountID, a.OidcURL),
				},
				Condition: map[string]any{
					"StringEquals": map[string]string{
						fmt.Sprintf("%s:sub", oidc): fmt.Sprintf("system:serviceaccount:%s:%s", namespaceName, accountName),
						fmt.Sprintf("%s:aud", oidc): "sts.amazonaws.com",
					},
				},
			},
		},
	}

	return policy
}

func (a *Agent) generateBaseTrustPolicyForRolesAnywhere(namespaceName string, accountName string) PolicyDocument {
	policy := PolicyDocument{
		Version: iamAPIVersion,
		Statement: []StatementEntry{
			{
				Effect: iamEffectAllow,
				Action: []string{"sts:AssumeRole", "sts:TagSession", "sts:SetSourceIdentity"},
				Principal: map[string]string{
					"Service": "rolesanywhere.amazonaws.com",
				},
				Condition: map[string]any{
					"StringEquals": map[string]string{
						"aws:PrincipalTag/x509SAN/URI": fmt.Sprintf("spiffe://%s/ns/%s/sa/%s", a.TrustDomain, namespaceName, accountName),
					},
					"ArnEquals": map[string]string{
						"aws:SourceArn": a.TrustAnchorArn,
					},
				},
			},
		},
	}

	return policy
}

func (a *Agent) generateRoleName(namespace string, accountName string) string {
	fullName := fmt.Sprintf("otr-%s.%s@%s", namespace, accountName, a.ClusterName)
	return agentutils.TruncateHashName(fullName, maxAWSNameLength)
}

func (a *Agent) generateRolesAnywhereProfileName(namespace string, accountName string) string {
	fullName := fmt.Sprintf("otr-%s_%s_%s", namespace, accountName, a.ClusterName)

	return agentutils.TruncateHashName(fullName, maxAWSNameLength)
}

func (a *Agent) GenerateRoleARN(namespace string, accountName string) string {
	roleName := a.generateRoleName(namespace, accountName)
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", a.AccountID, roleName)
}
