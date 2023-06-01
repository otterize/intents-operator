package istiopolicy

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/amit7itz/goset"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1beta1security "istio.io/api/security/v1beta1"
	v1beta1type "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"strings"
)

const (
	ReasonGettingIstioPolicyFailed  = "GettingIstioPolicyFailed"
	ReasonCreatingIstioPolicyFailed = "CreatingIstioPolicyFailed"
	ReasonUpdatingIstioPolicyFailed = "UpdatingIstioPolicyFailed"
	ReasonDeleteIstioPolicyFailed   = "DeleteIstioPolicyFailed"
	ReasonCreatedIstioPolicy        = "CreatedIstioPolicy"
	ReasonNamespaceNotAllowed       = "NamespaceNotAllowed"
	ReasonMissingSidecar            = "MissingSidecar"
	ReasonServerMissingSidecar      = "ServerMissingSidecar"
	ReasonSharedServiceAccount      = "SharedServiceAccountFound"
	OtterizeIstioPolicyNameTemplate = "authorization-policy-to-%s-from-%s"
)

type PolicyID types.UID

//+kubebuilder:rbac:groups="security.istio.io",resources=authorizationpolicies,verbs=get;update;patch;list;watch;delete;create;deletecollection
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents,verbs=get;list;watch;create;update;patch;delete

type AdminImpl struct {
	client               client.Client
	recorder             *injectablerecorder.InjectableRecorder
	restrictToNamespaces []string
}

type Admin interface {
	DeleteAll(ctx context.Context, clientIntents *v1alpha2.ClientIntents) error
	Create(ctx context.Context, clientIntents *v1alpha2.ClientIntents, clientServiceAccount string) error
	UpdateIntentsStatus(ctx context.Context, clientIntents *v1alpha2.ClientIntents, clientServiceAccount string, missingSideCar bool) error
	UpdateServerSidecar(ctx context.Context, clientIntents *v1alpha2.ClientIntents, serverName string, missingSideCar bool) error
}

func NewAdmin(client client.Client, recorder *injectablerecorder.InjectableRecorder, restrictedNamespaces []string) *AdminImpl {
	return &AdminImpl{
		client:               client,
		recorder:             recorder,
		restrictToNamespaces: restrictedNamespaces,
	}
}

func (c *AdminImpl) DeleteAll(
	ctx context.Context,
	clientIntents *v1alpha2.ClientIntents,
) error {
	clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(clientIntents.Spec.Service.Name, clientIntents.Namespace)

	var existingPolicies v1beta1.AuthorizationPolicyList
	err := c.client.List(ctx,
		&existingPolicies,
		client.MatchingLabels{v1alpha2.OtterizeIstioClientAnnotationKey: clientFormattedIdentity})
	if client.IgnoreNotFound(err) != nil {
		return err
	}

	for _, policy := range existingPolicies.Items {
		err = c.deletePolicy(ctx, policy)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *AdminImpl) deletePolicy(ctx context.Context, policy *v1beta1.AuthorizationPolicy) error {
	err := c.client.Delete(ctx, policy)
	if err != nil {
		return err
	}
	telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIstioPoliciesDeleted, 1)
	return nil
}

func (c *AdminImpl) Create(
	ctx context.Context,
	clientIntents *v1alpha2.ClientIntents,
	clientServiceAccount string,
) error {
	clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(clientIntents.Spec.Service.Name, clientIntents.Namespace)

	var existingPolicies v1beta1.AuthorizationPolicyList
	err := c.client.List(ctx,
		&existingPolicies,
		client.MatchingLabels{v1alpha2.OtterizeIstioClientAnnotationKey: clientFormattedIdentity})
	if err != nil {
		c.recorder.RecordWarningEventf(clientIntents, ReasonGettingIstioPolicyFailed, "Could not get Istio policies: %s", err.Error())
		return err
	}

	updatedPolicies, err := c.createOrUpdatePolicies(ctx, clientIntents, clientServiceAccount, existingPolicies)
	if err != nil {
		return err
	}

	err = c.deleteOutdatedPolicies(ctx, existingPolicies, updatedPolicies)
	if err != nil {
		c.recorder.RecordWarningEventf(clientIntents, ReasonDeleteIstioPolicyFailed, "Failed to delete Istio policy: %s", err.Error())
		return err
	}

	if len(clientIntents.GetCallsList()) > 0 {
		c.recorder.RecordNormalEventf(clientIntents, ReasonCreatedIstioPolicy, "Istio policy reconcile complete, reconciled %d servers", len(clientIntents.GetCallsList()))
	}

	return nil
}

func (c *AdminImpl) UpdateIntentsStatus(
	ctx context.Context,
	clientIntents *v1alpha2.ClientIntents,
	clientServiceAccount string,
	missingSideCar bool,
) error {
	err := c.saveServiceAccountName(ctx, clientIntents, clientServiceAccount)
	if err != nil {
		return err
	}

	err = c.saveSideCarStatus(ctx, clientIntents, missingSideCar)
	if err != nil {
		return err
	}

	return c.updateSharedServiceAccountsInNamespace(ctx, clientIntents.Namespace)
}

func (c *AdminImpl) UpdateServerSidecar(
	ctx context.Context,
	clientIntents *v1alpha2.ClientIntents,
	serverName string,
	missingSideCar bool,
) error {
	servers, err := clientIntents.GetServersWithoutSidecar()
	if err != nil {
		return err
	}
	intentsUpdateRequired := false
	if missingSideCar {
		if !servers.Has(serverName) {
			intentsUpdateRequired = true
		}
		servers.Insert(serverName)
	} else {
		if servers.Has(serverName) {
			intentsUpdateRequired = true
		}
		servers.Delete(serverName)
	}

	// If no update needed, skip intents update to avoid loop.
	if !intentsUpdateRequired {
		return nil
	}

	err = c.setServersWithoutSidecar(ctx, clientIntents, servers)
	if err != nil {
		return err
	}

	if missingSideCar {
		c.recorder.RecordWarningEventf(clientIntents, ReasonServerMissingSidecar, "Can't apply policies for server %s since it doesn't have sidecar", serverName)
	}

	return nil
}

func (c *AdminImpl) setServersWithoutSidecar(ctx context.Context, clientIntents *v1alpha2.ClientIntents, set sets.Set[string]) error {
	serversSortedList := sets.List(set)
	serversValues, err := json.Marshal(serversSortedList)
	if err != nil {
		return err
	}
	updatedIntents := clientIntents.DeepCopy()
	if updatedIntents.Annotations == nil {
		updatedIntents.Annotations = make(map[string]string)
	}

	updatedIntents.Annotations[v1alpha2.OtterizeServersWithoutSidecarAnnotation] = string(serversValues)
	err = c.client.Patch(ctx, updatedIntents, client.MergeFrom(clientIntents))
	if err != nil {
		return err
	}
	return nil
}

func (c *AdminImpl) saveServiceAccountName(ctx context.Context, clientIntents *v1alpha2.ClientIntents, clientServiceAccount string) error {
	serviceAccountLabelValue, ok := clientIntents.Annotations[v1alpha2.OtterizeClientServiceAccountAnnotation]
	if ok && serviceAccountLabelValue == clientServiceAccount {
		return nil
	}

	updatedIntents := clientIntents.DeepCopy()
	if updatedIntents.Annotations == nil {
		updatedIntents.Annotations = make(map[string]string)
	}

	updatedIntents.Annotations[v1alpha2.OtterizeClientServiceAccountAnnotation] = clientServiceAccount
	err := c.client.Patch(ctx, updatedIntents, client.MergeFrom(clientIntents))
	if err != nil {
		logrus.WithError(err).Errorln("Failed updating intent with service account name")
		return err
	}

	logrus.Infof("updating intent %s with service account name %s", clientIntents.Name, clientServiceAccount)

	return nil
}

func (c *AdminImpl) saveSideCarStatus(ctx context.Context, clientIntents *v1alpha2.ClientIntents, missingSideCar bool) error {
	oldValue, ok := clientIntents.Annotations[v1alpha2.OtterizeMissingSidecarAnnotation]
	if ok && oldValue == strconv.FormatBool(missingSideCar) {
		return nil
	}

	updatedIntents := clientIntents.DeepCopy()
	if updatedIntents.Annotations == nil {
		updatedIntents.Annotations = make(map[string]string)
	}

	updatedIntents.Annotations[v1alpha2.OtterizeMissingSidecarAnnotation] = strconv.FormatBool(missingSideCar)
	err := c.client.Patch(ctx, updatedIntents, client.MergeFrom(clientIntents))
	if err != nil {
		return err
	}

	return nil
}

func (c *AdminImpl) updateSharedServiceAccountsInNamespace(ctx context.Context, namespace string) error {
	var namespacesClientIntents v1alpha2.ClientIntentsList
	err := c.client.List(
		ctx, &namespacesClientIntents,
		&client.ListOptions{Namespace: namespace})
	if err != nil {
		return err
	}

	clientsByServiceAccount := lo.GroupBy(namespacesClientIntents.Items, func(intents v1alpha2.ClientIntents) string {
		return intents.Annotations[v1alpha2.OtterizeClientServiceAccountAnnotation]
	})

	for clientServiceAccountName, clientIntents := range clientsByServiceAccount {
		err = c.updateServiceAccountSharedStatus(ctx, clientIntents, clientServiceAccountName)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *AdminImpl) updateServiceAccountSharedStatus(ctx context.Context, clientIntents []v1alpha2.ClientIntents, serviceAccount string) error {
	logrus.Infof("Found %d intents with service account %s", len(clientIntents), serviceAccount)
	isServiceAccountShared := len(clientIntents) > 1
	sharedAccountValue := lo.Ternary(isServiceAccountShared, "true", "false")

	clients := lo.Map(clientIntents, func(intents v1alpha2.ClientIntents, _ int) string {
		return intents.Spec.Service.Name
	})
	clientsNames := strings.Join(clients, ", ")

	for _, intents := range clientIntents {
		if !shouldUpdateStatus(intents, serviceAccount, sharedAccountValue) {
			continue
		}

		updatedIntents := intents.DeepCopy()
		if updatedIntents.Annotations == nil {
			updatedIntents.Annotations = make(map[string]string)
		}
		updatedIntents.Annotations[v1alpha2.OtterizeSharedServiceAccountAnnotation] = sharedAccountValue
		err := c.client.Patch(ctx, updatedIntents, client.MergeFrom(&intents))
		if err != nil {
			return err
		}

		if isServiceAccountShared {
			c.recorder.RecordWarningEventf(updatedIntents, ReasonSharedServiceAccount, "Service account %s is shared and will also grant access to the following clients: %s", serviceAccount, clientsNames)
		}
	}
	return nil
}

func shouldUpdateStatus(intents v1alpha2.ClientIntents, currentName string, currentSharedStatus string) bool {
	oldName, annotationExists := intents.Annotations[v1alpha2.OtterizeClientServiceAccountAnnotation]
	if !annotationExists || oldName != currentName {
		return true
	}

	oldSharedStatus, annotationExists := intents.Annotations[v1alpha2.OtterizeSharedServiceAccountAnnotation]
	if !annotationExists || oldSharedStatus != currentSharedStatus {
		return true
	}

	return false
}

func (c *AdminImpl) createOrUpdatePolicies(
	ctx context.Context,
	clientIntents *v1alpha2.ClientIntents,
	clientServiceAccount string,
	existingPolicies v1beta1.AuthorizationPolicyList,
) (goset.Set[PolicyID], error) {
	updatedPolicies := goset.NewSet[PolicyID]()
	for _, intent := range clientIntents.GetCallsList() {
		targetNamespace := intent.GetServerNamespace(clientIntents.Namespace)
		if len(c.restrictToNamespaces) != 0 && !lo.Contains(c.restrictToNamespaces, targetNamespace) {
			c.recorder.RecordWarningEventf(
				clientIntents,
				ReasonNamespaceNotAllowed,
				"Namespace %s was specified in intent, but is not allowed by configuration, Istio policy ignored",
				targetNamespace,
			)
			continue
		}

		newPolicy := c.generateAuthorizationPolicy(clientIntents, intent, clientServiceAccount)
		existingPolicy, found := c.findPolicy(existingPolicies, newPolicy)
		if found {
			err := c.updatePolicy(ctx, existingPolicy, newPolicy)
			if err != nil {
				c.recorder.RecordWarningEventf(clientIntents, ReasonUpdatingIstioPolicyFailed, "Failed to update Istio policy: %s", err.Error())
				return goset.Set[PolicyID]{}, err
			}
			updatedPolicies.Add(PolicyID(existingPolicy.UID))
			continue
		}

		err := c.client.Create(ctx, newPolicy)
		if err != nil {
			c.recorder.RecordWarningEventf(clientIntents, ReasonCreatingIstioPolicyFailed, "Failed to create Istio policy: %s", err.Error())
			return goset.Set[PolicyID]{}, err
		}
		telemetrysender.SendIntentOperator(telemetriesgql.EventTypeIstioPoliciesCreated, 1)
	}

	return *updatedPolicies, nil
}

func (c *AdminImpl) findPolicy(existingPolicies v1beta1.AuthorizationPolicyList, newPolicy *v1beta1.AuthorizationPolicy) (*v1beta1.AuthorizationPolicy, bool) {
	for _, policy := range existingPolicies.Items {
		if policy.Labels[v1alpha2.OtterizeServerLabelKey] == newPolicy.Labels[v1alpha2.OtterizeServerLabelKey] {
			return policy, true
		}
	}
	return nil, false
}

func (c *AdminImpl) deleteOutdatedPolicies(ctx context.Context, existingPolicies v1beta1.AuthorizationPolicyList, validPolicies goset.Set[PolicyID]) error {
	for _, existingPolicy := range existingPolicies.Items {
		if !validPolicies.Contains(PolicyID(existingPolicy.UID)) {
			err := c.deletePolicy(ctx, existingPolicy)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *AdminImpl) updatePolicy(ctx context.Context, existingPolicy *v1beta1.AuthorizationPolicy, newPolicy *v1beta1.AuthorizationPolicy) error {
	if c.isPolicyEqual(existingPolicy, newPolicy) {
		return nil
	}

	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Spec.Rules = newPolicy.Spec.Rules
	policyCopy.Spec.Selector = newPolicy.Spec.Selector

	err := c.client.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
	if err != nil {
		c.recorder.RecordWarningEventf(existingPolicy, ReasonUpdatingIstioPolicyFailed, "Failed to update Istio policy: %s", err.Error())
		return err
	}

	return nil
}

func (c *AdminImpl) getPolicyName(intents *v1alpha2.ClientIntents, intent v1alpha2.Intent) string {
	clientName := fmt.Sprintf("%s.%s", intents.GetServiceName(), intents.Namespace)
	policyName := fmt.Sprintf(OtterizeIstioPolicyNameTemplate, intent.GetServerName(), clientName)
	return policyName
}

func (c *AdminImpl) isPolicyEqual(existingPolicy *v1beta1.AuthorizationPolicy, newPolicy *v1beta1.AuthorizationPolicy) bool {
	sameServer := existingPolicy.Spec.Selector.MatchLabels[v1alpha2.OtterizeServerLabelKey] == newPolicy.Spec.Selector.MatchLabels[v1alpha2.OtterizeServerLabelKey]
	samePrincipals := existingPolicy.Spec.Rules[0].From[0].Source.Principals[0] == newPolicy.Spec.Rules[0].From[0].Source.Principals[0]
	sameHTTPRules := compareHTTPRules(existingPolicy.Spec.Rules[0].To, newPolicy.Spec.Rules[0].To)

	return sameServer && samePrincipals && sameHTTPRules
}

func compareHTTPRules(existingRules []*v1beta1security.Rule_To, newRules []*v1beta1security.Rule_To) bool {
	if len(existingRules) == 0 && len(newRules) == 0 {
		return true
	}

	if len(existingRules) != len(newRules) {
		return false
	}

	for i := range existingRules {
		existingOperation := existingRules[i].Operation
		newOperation := newRules[i].Operation
		if existingOperation.Paths[0] != newOperation.Paths[0] {
			return false
		}

		for j, method := range existingOperation.Methods {
			if method != newOperation.Methods[j] {
				return false
			}
		}
	}

	return true
}

func (c *AdminImpl) generateAuthorizationPolicy(
	clientIntents *v1alpha2.ClientIntents,
	intent v1alpha2.Intent,
	clientServiceAccountName string,
) *v1beta1.AuthorizationPolicy {
	policyName := c.getPolicyName(clientIntents, intent)
	logrus.Infof("Creating Istio policy %s for intent %s", policyName, intent.GetServerName())

	serverNamespace := intent.GetServerNamespace(clientIntents.Namespace)
	formattedTargetServer := v1alpha2.GetFormattedOtterizeIdentity(intent.GetServerName(), serverNamespace)
	clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(clientIntents.GetServiceName(), clientIntents.Namespace)

	var ruleTo []*v1beta1security.Rule_To
	if intent.Type == v1alpha2.IntentTypeHTTP {
		ruleTo = make([]*v1beta1security.Rule_To, 0)
		operations := c.intentsHTTPResourceToIstioOperations(intent.HTTPResources)
		for _, operation := range operations {
			ruleTo = append(ruleTo, &v1beta1security.Rule_To{
				Operation: operation,
			})
		}
	}

	source := fmt.Sprintf("cluster.local/ns/%s/sa/%s", clientIntents.Namespace, clientServiceAccountName)
	newPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: serverNamespace,
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           formattedTargetServer,
				v1alpha2.OtterizeIstioClientAnnotationKey: clientFormattedIdentity,
			},
		},
		Spec: v1beta1security.AuthorizationPolicy{
			Selector: &v1beta1type.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: formattedTargetServer,
				},
			},
			Action: v1beta1security.AuthorizationPolicy_ALLOW,
			Rules: []*v1beta1security.Rule{
				{
					To: ruleTo,
					From: []*v1beta1security.Rule_From{
						{
							Source: &v1beta1security.Source{
								Principals: []string{
									source,
								},
							},
						},
					},
				},
			},
		},
	}

	return newPolicy
}

func (c *AdminImpl) intentsHTTPResourceToIstioOperations(resources []v1alpha2.HTTPResource) []*v1beta1security.Operation {
	operations := make([]*v1beta1security.Operation, 0, len(resources))

	for _, resource := range resources {
		operations = append(operations, &v1beta1security.Operation{
			Methods: c.intentsMethodsToIstioMethods(resource.Methods),
			Paths:   []string{resource.Path},
		})
	}

	return operations
}

func (c *AdminImpl) intentsMethodsToIstioMethods(intent []v1alpha2.HTTPMethod) []string {
	istioMethods := make([]string, 0, len(intent))
	for _, method := range intent {
		// Istio documentation specifies "A list of methods as specified in the HTTP request" in uppercase
		istioMethods = append(istioMethods, string(method))
	}

	return istioMethods
}
