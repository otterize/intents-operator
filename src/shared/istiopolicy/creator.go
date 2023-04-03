package istiopolicy

import (
	"github.com/amit7itz/goset"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1beta12 "istio.io/api/security/v1beta1"
	v1beta13 "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ReasonGettingIstioPolicyFailed  = "GettingIstioPolicyFailed"
	ReasonCreatingIstioPolicyFailed = "CreatingIstioPolicyFailed"
	ReasonUpdatingIstioPolicyFailed = "UpdatingIstioPolicyFailed"
	ReasonDeleteIstioPolicyFailed   = "DeleteIstioPolicyFailed"
	ReasonCreatedIstioPolicy        = "CreatedIstioPolicy"
	ReasonNamespaceNotAllowed       = "NamespaceNotAllowed"
	OtterizeIstioPolicyNameTemplate = "authorization-policy-to-%s-from-%s"
)

type PolicyID types.UID

//+kubebuilder:rbac:groups="security.istio.io",resources=authorizationpolicies,verbs=get;update;patch;list;watch;delete;create

type Creator struct {
	client               client.Client
	recorder             *injectablerecorder.InjectableRecorder
	restrictToNamespaces []string
}

func NewCreator(client client.Client, recorder *injectablerecorder.InjectableRecorder, restrictedNamespaces []string) *Creator {
	return &Creator{
		client:               client,
		recorder:             recorder,
		restrictToNamespaces: restrictedNamespaces,
	}
}

func (c *Creator) DeleteAll(
	ctx context.Context,
	clientIntents *v1alpha2.ClientIntents,
) error {
	clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(clientIntents.Spec.Service.Name, clientIntents.Namespace)

	err := c.client.DeleteAllOf(ctx,
		&v1beta1.AuthorizationPolicy{},
		client.MatchingLabels{v1alpha2.OtterizeIstioClientAnnotationKey: clientFormattedIdentity})
	if err != nil && !k8serrors.IsNotFound(err) {
		c.recorder.RecordWarningEventf(clientIntents, ReasonGettingIstioPolicyFailed, "could not get istio policies: %s", err.Error())
		return err
	}

	return nil
}

func (c *Creator) Create(
	ctx context.Context,
	clientIntents *v1alpha2.ClientIntents,
	clientNamespace string,
	clientServiceAccount string,
) error {
	clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(clientIntents.Spec.Service.Name, clientNamespace)

	var existingPolicies v1beta1.AuthorizationPolicyList
	err := c.client.List(ctx,
		&existingPolicies,
		client.MatchingLabels{v1alpha2.OtterizeIstioClientAnnotationKey: clientFormattedIdentity})
	if err != nil {
		c.recorder.RecordWarningEventf(clientIntents, ReasonGettingIstioPolicyFailed, "could not get istio policies: %s", err.Error())
		return err
	}

	updatedPolicies, err := c.createOrUpdatePolicies(ctx, clientIntents, clientNamespace, clientServiceAccount, existingPolicies)
	if err != nil {
		return err
	}

	err = c.deleteOutdatedPolicies(ctx, existingPolicies, updatedPolicies)
	if err != nil {
		c.recorder.RecordWarningEventf(clientIntents, ReasonDeleteIstioPolicyFailed, "failed to delete istio policy: %s", err.Error())
		return err
	}

	if len(clientIntents.GetCallsList()) > 0 {
		c.recorder.RecordNormalEventf(clientIntents, ReasonCreatedIstioPolicy, "istio policies reconcile complete, reconciled %d servers", len(clientIntents.GetCallsList()))
	}

	return nil
}

func (c *Creator) createOrUpdatePolicies(
	ctx context.Context,
	clientIntents *v1alpha2.ClientIntents,
	clientNamespace string,
	clientServiceAccount string,
	existingPolicies v1beta1.AuthorizationPolicyList,
) (goset.Set[PolicyID], error) {
	updatedPolicies := goset.NewSet[PolicyID]()
	for _, intent := range clientIntents.GetCallsList() {
		if c.namespaceNotAllowed(intent, clientNamespace) {
			c.recorder.RecordWarningEventf(
				clientIntents,
				ReasonNamespaceNotAllowed,
				"namespace %s was specified in intent, but is not allowed by configuration, istio policy ignored",
				clientNamespace,
			)
			continue
		}

		newPolicy := c.generateAuthorizationPolicy(clientIntents, intent, clientNamespace, clientServiceAccount)
		existingPolicy, found := c.findPolicy(existingPolicies, newPolicy)
		if found {
			err := c.UpdatePolicy(ctx, existingPolicy, newPolicy)
			if err != nil {
				c.recorder.RecordWarningEventf(clientIntents, ReasonUpdatingIstioPolicyFailed, "failed to update istio policy: %s", err.Error())
				return goset.Set[PolicyID]{}, err
			}
			updatedPolicies.Add(PolicyID(existingPolicy.UID))
			continue
		}

		err := c.client.Create(ctx, newPolicy)
		if err != nil {
			c.recorder.RecordWarningEventf(clientIntents, ReasonCreatingIstioPolicyFailed, "failed to istio policy: %s", err.Error())
			return goset.Set[PolicyID]{}, err
		}
	}

	return *updatedPolicies, nil
}

func (c *Creator) findPolicy(existingPolicies v1beta1.AuthorizationPolicyList, newPolicy *v1beta1.AuthorizationPolicy) (*v1beta1.AuthorizationPolicy, bool) {
	for _, policy := range existingPolicies.Items {
		if policy.Labels[v1alpha2.OtterizeServerLabelKey] == newPolicy.Labels[v1alpha2.OtterizeServerLabelKey] {
			return policy, true
		}
	}
	return nil, false
}

func (c *Creator) deleteOutdatedPolicies(ctx context.Context, existingPolicies v1beta1.AuthorizationPolicyList, validPolicies goset.Set[PolicyID]) error {
	for _, existingPolicy := range existingPolicies.Items {
		if !validPolicies.Contains(PolicyID(existingPolicy.UID)) {
			err := c.client.Delete(ctx, existingPolicy)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Creator) namespaceNotAllowed(intent v1alpha2.Intent, requestNamespace string) bool {
	targetNamespace := intent.GetServerNamespace(requestNamespace)
	restrictedNamespacesExists := len(c.restrictToNamespaces) != 0
	return restrictedNamespacesExists && !lo.Contains(c.restrictToNamespaces, targetNamespace)
}

func (c *Creator) UpdatePolicy(ctx context.Context, existingPolicy *v1beta1.AuthorizationPolicy, newPolicy *v1beta1.AuthorizationPolicy) error {
	if c.isPolicyEqual(existingPolicy, newPolicy) {
		return nil
	}

	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Spec.Rules = newPolicy.Spec.Rules
	policyCopy.Spec.Selector = newPolicy.Spec.Selector

	err := c.client.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
	if err != nil {
		c.recorder.RecordWarningEventf(existingPolicy, ReasonUpdatingIstioPolicyFailed, "failed to update istio policy: %s", err.Error())
		return err
	}
	logrus.Infof("Updating existing istio policy %s", newPolicy.Name)

	return nil
}

func (c *Creator) getPolicyName(intents *v1alpha2.ClientIntents, intent v1alpha2.Intent) string {
	clientName := fmt.Sprintf("%s.%s", intents.GetServiceName(), intents.Namespace)
	policyName := fmt.Sprintf(OtterizeIstioPolicyNameTemplate, intent.GetServerName(), clientName)
	return policyName
}

func (c *Creator) isPolicyEqual(existingPolicy *v1beta1.AuthorizationPolicy, newPolicy *v1beta1.AuthorizationPolicy) bool {
	sameServer := existingPolicy.Spec.Selector.MatchLabels[v1alpha2.OtterizeServerLabelKey] == newPolicy.Spec.Selector.MatchLabels[v1alpha2.OtterizeServerLabelKey]
	samePrincipals := existingPolicy.Spec.Rules[0].From[0].Source.Principals[0] == newPolicy.Spec.Rules[0].From[0].Source.Principals[0]
	sameHTTPRules := compareHTTPRules(existingPolicy.Spec.Rules[0].To, newPolicy.Spec.Rules[0].To)

	return sameServer && samePrincipals && sameHTTPRules
}

func compareHTTPRules(existingRules []*v1beta12.Rule_To, newRules []*v1beta12.Rule_To) bool {
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

func (c *Creator) generateAuthorizationPolicy(
	clientIntents *v1alpha2.ClientIntents,
	intent v1alpha2.Intent,
	objectNamespace string,
	clientServiceAccountName string,
) *v1beta1.AuthorizationPolicy {
	policyName := c.getPolicyName(clientIntents, intent)
	logrus.Infof("Creating istio policy %s for intent %s", policyName, intent.GetServerName())

	serverNamespace := intent.GetServerNamespace(objectNamespace)
	formattedTargetServer := v1alpha2.GetFormattedOtterizeIdentity(intent.GetServerName(), serverNamespace)
	clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(clientIntents.Spec.Service.Name, objectNamespace)

	var ruleTo []*v1beta12.Rule_To
	if intent.Type == v1alpha2.IntentTypeHTTP {
		ruleTo = make([]*v1beta12.Rule_To, 0)
		operations := c.intentsHTTPResourceToIstioOperations(intent.HTTPResources)
		for _, operation := range operations {
			ruleTo = append(ruleTo, &v1beta12.Rule_To{
				Operation: operation,
			})
		}
	}

	source := fmt.Sprintf("cluster.local/ns/%s/sa/%s", objectNamespace, clientServiceAccountName)
	newPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: serverNamespace,
			Labels: map[string]string{
				v1alpha2.OtterizeServerLabelKey:           formattedTargetServer,
				v1alpha2.OtterizeIstioClientAnnotationKey: clientFormattedIdentity,
			},
		},
		Spec: v1beta12.AuthorizationPolicy{
			Selector: &v1beta13.WorkloadSelector{
				MatchLabels: map[string]string{
					v1alpha2.OtterizeServerLabelKey: formattedTargetServer,
				},
			},
			Action: v1beta12.AuthorizationPolicy_ALLOW,
			Rules: []*v1beta12.Rule{
				{
					To: ruleTo,
					From: []*v1beta12.Rule_From{
						{
							Source: &v1beta12.Source{
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

func (c *Creator) intentsHTTPResourceToIstioOperations(resources []v1alpha2.HTTPResource) []*v1beta12.Operation {
	operations := make([]*v1beta12.Operation, 0, len(resources))

	for _, resource := range resources {
		operations = append(operations, &v1beta12.Operation{
			Methods: c.intentsMethodsToIstioMethods(resource.Methods),
			Paths:   []string{resource.Path},
		})
	}

	return operations
}

func (c *Creator) intentsMethodsToIstioMethods(intent []v1alpha2.HTTPMethod) []string {
	istioMethods := make([]string, 0, len(intent))
	for _, method := range intent {
		// Istio documentation specifies "A list of methods as specified in the HTTP request" in uppercase
		istioMethods = append(istioMethods, string(method))
	}

	return istioMethods
}
