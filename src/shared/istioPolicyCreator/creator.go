package istiopolicy

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	v1beta12 "istio.io/api/security/v1beta1"
	v1beta13 "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ReasonGettingIstioPolicyFailed  = "GettingIstioPolicyFailed"
	ReasonCreatingIstioPolicyFailed = "CreatingIstioPolicyFailed"
	ReasonUpdatingIstioPolicyFailed = "UpdatingIstioPolicyFailed"
	ReasonCreatedIstioPolicy        = "CreatedIstioPolicy"
	ReasonNamespaceNotAllowed       = "NamespaceNotAllowed"
	OtterizeIstioPolicyNameTemplate = "authorization-policy-to-%s-from-%s"
)

type Creator struct {
	client.Client
	injectablerecorder.InjectableRecorder
	RestrictToNamespaces []string
}

func CreatePolicy(
	ctx context.Context,
	client client.Client,
	recorder injectablerecorder.InjectableRecorder,
	intents *v1alpha2.ClientIntents,
	RestrictToNamespaces []string,
	clientIntentsNamespace string,
	clientServiceAccountName string,
) error {
	creator := Creator{
		Client:               client,
		InjectableRecorder:   recorder,
		RestrictToNamespaces: RestrictToNamespaces,
	}

	return creator.handleAuthorizationPolicy(ctx, intents, clientIntentsNamespace, clientServiceAccountName)
}

func (r *Creator) handleAuthorizationPolicy(
	ctx context.Context,
	intents *v1alpha2.ClientIntents,
	clientIntentsNamespace string,
	clientServiceAccountName string,
) error {
	for _, intent := range intents.GetCallsList() {
		if r.namespaceNotAllowed(intent, clientIntentsNamespace) {
			r.RecordWarningEventf(
				intents,
				ReasonNamespaceNotAllowed,
				"namespace %s was specified in intent, but is not allowed by configuration, istio policy ignored",
				clientIntentsNamespace,
			)
			continue
		}
		err := r.updateOrCreatePolicy(ctx, intents, intent, clientIntentsNamespace, clientServiceAccountName)
		if err != nil {
			r.RecordWarningEventf(intents, ReasonCreatingIstioPolicyFailed, "could not create istio policies: %s", err.Error())
			return err
		}
	}

	if len(intents.GetCallsList()) > 0 {
		r.RecordNormalEventf(intents, ReasonCreatedIstioPolicy, "istio policies reconcile complete, reconciled %d servers", len(intents.GetCallsList()))
	}

	return nil
}

func (r *Creator) namespaceNotAllowed(intent v1alpha2.Intent, requestNamespace string) bool {
	targetNamespace := intent.GetServerNamespace(requestNamespace)
	restrictedNamespacesExists := len(r.RestrictToNamespaces) != 0
	return restrictedNamespacesExists && !lo.Contains(r.RestrictToNamespaces, targetNamespace)
}

func (r *Creator) updateOrCreatePolicy(
	ctx context.Context,
	intents *v1alpha2.ClientIntents,
	intent v1alpha2.Intent,
	objectNamespace string,
	clientServiceAccountName string,
) error {
	clientName := intents.Spec.Service.Name
	policyName := fmt.Sprintf(OtterizeIstioPolicyNameTemplate, intent.GetServerName(), clientName)
	newPolicy := r.getAuthorizationPolicyForIntent(intent, objectNamespace, policyName, clientServiceAccountName)

	existingPolicy := &v1beta1.AuthorizationPolicy{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      policyName,
		Namespace: intent.GetServerNamespace(objectNamespace)},
		existingPolicy)
	if err != nil && !errors.IsNotFound(err) {
		r.RecordWarningEventf(existingPolicy, ReasonGettingIstioPolicyFailed, "failed to get istio policy: %s", err.Error())
		return err
	}

	if errors.IsNotFound(err) {
		err = r.Create(ctx, newPolicy)
		if err != nil {
			r.RecordWarningEventf(existingPolicy, ReasonCreatingIstioPolicyFailed, "failed to istio policy: %s", err.Error())
			return err
		}
		return nil
	}

	logrus.Infof("Found existing istio policy %s", policyName)

	if !r.isPolicyEqual(existingPolicy, newPolicy) {
		logrus.Infof("Updating existing istio policy %s", policyName)
		policyCopy := existingPolicy.DeepCopy()
		policyCopy.Spec.Rules[0].From[0].Source.Principals[0] = newPolicy.Spec.Rules[0].From[0].Source.Principals[0]
		policyCopy.Spec.Selector.MatchLabels[v1alpha2.OtterizeServerLabelKey] = newPolicy.Spec.Selector.MatchLabels[v1alpha2.OtterizeServerLabelKey]
		err = r.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
		if err != nil {
			r.RecordWarningEventf(existingPolicy, ReasonUpdatingIstioPolicyFailed, "failed to update istio policy: %s", err.Error())
			return err
		}
	}

	return nil
}

func (r *Creator) isPolicyEqual(existingPolicy *v1beta1.AuthorizationPolicy, newPolicy *v1beta1.AuthorizationPolicy) bool {
	sameServer := existingPolicy.Spec.Selector.MatchLabels[v1alpha2.OtterizeServerLabelKey] == newPolicy.Spec.Selector.MatchLabels[v1alpha2.OtterizeServerLabelKey]
	samePrincipal := existingPolicy.Spec.Rules[0].From[0].Source.Principals[0] == newPolicy.Spec.Rules[0].From[0].Source.Principals[0]
	return sameServer && samePrincipal
}

func (r *Creator) getAuthorizationPolicyForIntent(
	intent v1alpha2.Intent,
	objectNamespace string,
	policyName string,
	clientServiceAccountName string,
) *v1beta1.AuthorizationPolicy {
	logrus.Infof("Creating istio policy %s for intent %s", policyName, intent.GetServerName())

	serverNamespace := intent.GetServerNamespace(objectNamespace)
	formattedTargetServer := v1alpha2.GetFormattedOtterizeIdentity(intent.GetServerName(), serverNamespace)

	source := fmt.Sprintf("cluster.local/ns/%s/sa/%s", objectNamespace, clientServiceAccountName)
	newPolicy := &v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: serverNamespace,
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
