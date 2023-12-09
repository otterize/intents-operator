package linkerdmanager

import (
	"context"
	"fmt"

	"github.com/amit7itz/goset"
	authpolicy "github.com/linkerd/linkerd2/controller/gen/apis/policy/v1alpha1"
	linkerdserver "github.com/linkerd/linkerd2/controller/gen/apis/server/v1beta1"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/protected_services"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapiv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
)

type PolicyID types.UID

const (
	ReasonGettingLinkerdPolicyFailed      = "GettingLinkerdPolicyFailed"
	OtterizeLinkerdServerNameTemplate     = "server-for-service-%s-port-%d"
	OtterizeLinkerdMeshTLSNameTemplate    = "meshtls-%s"
	OtterizeLinkerdAuthPolicyNameTemplate = "authorization-policy-to-%s-using-%s"
	ReasonDeleteLinkerdPolicyFailed       = "DeleteLinkerdPolicyFailed"
	ReasonNamespaceNotAllowed             = "NamespaceNotAllowed"
	ReasonLinkerdPolicy                   = "LinkerdPolicy"
	ReasonMissingSidecar                  = "MissingSideCar"
	ReasonCreatingLinkerdPolicyFailed     = "CreatingLinkerdPolicyFailed"
	ReasonUpdatingLinkerdPolicyFailed     = "UpdatingLinkerdPolicyFailed"
)

type LinkerdPolicyManager interface {
	DeleteAll(ctx context.Context, clientIntents *v1alpha3.ClientIntents) error
	Create(ctx context.Context, clientIntents *v1alpha3.ClientIntents, clientServiceAccount string) error
	UpdateIntentsStatus(ctx context.Context, clientIntents *v1alpha3.ClientIntents, clientServiceAccount string, missingSideCar bool) error
	UpdateServerSidecar(ctx context.Context, clientIntents *v1alpha3.ClientIntents, serverName string, missingSideCar bool) error
}

type LinkerdManager struct {
	client.Client
	serviceIdResolver serviceidresolver.ServiceResolver
	injectablerecorder.InjectableRecorder
	restrictedToNamespaces      []string
	enforcementDefaultState     bool
	enableLinkerdPolicyCreation bool
}

func NewLinkerdManager(c client.Client,
	namespaces []string,
	enforcementDefaultState,
	enableLinkerdPolicyCreation bool) *LinkerdManager {
	return &LinkerdManager{
		Client:                      c,
		serviceIdResolver:           serviceidresolver.NewResolver(c),
		restrictedToNamespaces:      namespaces,
		enforcementDefaultState:     enforcementDefaultState,
		enableLinkerdPolicyCreation: enableLinkerdPolicyCreation,
	}
}

func (ldm *LinkerdManager) Create(
	ctx context.Context,
	clientIntents *v1alpha3.ClientIntents,
	clientServiceAccount string,
) error {
	clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(clientIntents.Spec.Service.Name, clientIntents.Namespace)

	var existingPolicies authpolicy.AuthorizationPolicyList
	err := ldm.Client.List(ctx,
		&existingPolicies,
		client.MatchingLabels{v1alpha3.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd policies: %s", err.Error())
		return err
	}

	_, err = ldm.createPolicies(ctx, clientIntents, clientServiceAccount, existingPolicies)
	if err != nil {
		return err
	}
	return nil
}

func (ldm *LinkerdManager) createPolicies(
	ctx context.Context,
	clientIntents *v1alpha3.ClientIntents,
	clientServiceAccount string, // supplied in the create method
	existingPolicies authpolicy.AuthorizationPolicyList,
) (*goset.Set[PolicyID], error) {
	updatedPolicies := goset.NewSet[PolicyID]()
	createdAnyPolicies := false
	for _, intent := range clientIntents.GetCallsList() {
		if intent.Type != "" && intent.Type != v1alpha3.IntentTypeHTTP {
			continue
		}
		shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState( //TODO:  check what that does
			ctx, ldm.Client, intent.GetTargetServerName(), intent.GetTargetServerNamespace(clientIntents.Namespace), ldm.enforcementDefaultState)
		if err != nil {
			return nil, err
		}

		if !shouldCreatePolicy {
			logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for server %s in namespace %s", intent.GetTargetServerName(), intent.GetTargetServerNamespace(clientIntents.Namespace))
			ldm.RecordNormalEventf(clientIntents, consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally and called service '%s' is not explicitly protected using a ProtectedService resource, network policy creation skipped", intent.Name)
			continue
		}

		if !ldm.enableLinkerdPolicyCreation {
			ldm.RecordNormalEvent(clientIntents, consts.ReasonIstioPolicyCreationDisabled, "Linkerd policy creation is disabled, creation skipped")
			return updatedPolicies, nil
		}

		targetNamespace := intent.GetTargetServerNamespace(clientIntents.Namespace)
		if len(ldm.restrictedToNamespaces) != 0 && !lo.Contains(ldm.restrictedToNamespaces, targetNamespace) {
			ldm.RecordWarningEventf(
				clientIntents,
				ReasonNamespaceNotAllowed,
				"Namespace %s was specified in intent, but is not allowed by configuration, Linkerd policy ignored",
				targetNamespace,
			)
			continue
		}

		pod, err := ldm.serviceIdResolver.ResolveIntentServerToPod(ctx, intent, clientIntents.Namespace)
		if err != nil {
			return nil, err
		}

		// check if there's a server for that service
		s, shouldCreateServer, err := ldm.shouldCreateServer(ctx, *clientIntents, intent)
		if err != nil {
			return nil, err
		}

		if shouldCreateServer {
			port := pod.Spec.Containers[0].Ports[0].HostPort // get proper port
			podSelector := ldm.BuildPodLabelSelectorFromIntent(intent, clientIntents.Namespace)

			s = ldm.generateLinkerdServer(*clientIntents, intent, podSelector, port)

			err = ldm.Client.Create(ctx, s)
			if err != nil {
				ldm.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd server: %s", err.Error())
				return nil, err
			}
		}

		shouldCreatePolicyAndMeshTLS, err := ldm.shouldCreateAuthPolicy(ctx, *clientIntents, s.Name, clientServiceAccount)
		if err != nil {
			return nil, err
		}

		if shouldCreatePolicyAndMeshTLS {
			mtls := ldm.generateMeshTLS(*clientIntents, intent, []string{clientServiceAccount})
			newPolicy := ldm.generateAuthorizationPolicy(*clientIntents, s.Name, mtls.Name)

			err = ldm.Client.Create(ctx, mtls)
			if err != nil {
				ldm.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd meshTLS: %s", err.Error())
				return nil, err
			}

			err = ldm.Client.Create(ctx, newPolicy)
			if err != nil {
				ldm.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd policy: %s", err.Error())
				return nil, err
			}
			createdAnyPolicies = true
		}
	}

	if updatedPolicies.Len() != 0 || createdAnyPolicies { // TODO: understand this
		ldm.RecordNormalEventf(clientIntents, ReasonLinkerdPolicy, "Linkerd policy reconcile complete, reconciled %d servers", len(clientIntents.GetCallsList()))
	}

	return updatedPolicies, nil
}

func (ldm *LinkerdManager) BuildPodLabelSelectorFromIntent(intent otterizev1alpha3.Intent, intentsObjNamespace string) metav1.LabelSelector {
	otterizeServerLabel := map[string]string{}
	targetNamespace := intent.GetTargetServerNamespace(intentsObjNamespace)
	formattedTargetServer := otterizev1alpha3.GetFormattedOtterizeIdentity(intent.GetTargetServerName(), targetNamespace)
	otterizeServerLabel[otterizev1alpha3.OtterizeServerLabelKey] = formattedTargetServer

	return metav1.LabelSelector{MatchLabels: otterizeServerLabel}
}

func (ldm *LinkerdManager) getServerName(intent otterizev1alpha3.Intent, port int32) string {
	name := intent.GetTargetServerName()
	return fmt.Sprintf(OtterizeLinkerdServerNameTemplate, name, port)
}

func (ldm *LinkerdManager) shouldCreateServer(ctx context.Context, intents otterizev1alpha3.ClientIntents, intent otterizev1alpha3.Intent) (*linkerdserver.Server, bool, error) {
	pod, err := ldm.serviceIdResolver.ResolveIntentServerToPod(ctx, intent, intents.Namespace)
	if err != nil {
		return nil, true, err
	}

	linkerdServerServiceFormattedIdentity := v1alpha3.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)
	servers := &linkerdserver.ServerList{}
	err = ldm.Client.List(ctx, servers, client.MatchingLabels{v1alpha3.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity})
	if err != nil {
		return nil, false, err
	}

	// no servers exist
	if len(servers.Items) == 0 {
		return nil, true, nil
	}

	// get servers in the namespace and if any of them has a label selector similar to the intents label return that server
	for _, server := range servers.Items {
		for _, container := range pod.Spec.Containers {
			for _, port := range container.Ports {
				if port.HostPort == server.Spec.Port.IntVal {
					return &server, false, nil
				}
			}
		}
	}
	return nil, true, nil
}

func (ldm *LinkerdManager) shouldCreateAuthPolicy(ctx context.Context, intents otterizev1alpha3.ClientIntents, targetServer, targetClient string) (bool, error) {
	// check if this service exists in a meshtls that is applied to an auth policy that works for that server
	// list auth policies for a server and if one matches check the meshtls name
	authPolicies := &authpolicy.AuthorizationPolicyList{}

	err := ldm.Client.List(ctx, authPolicies, &client.ListOptions{Namespace: intents.Namespace})
	if err != nil {
		return false, err
	}
	for _, policy := range authPolicies.Items {
		if policy.Spec.TargetRef.Name == v1beta1.ObjectName(targetServer) && policy.Spec.TargetRef.Kind == "Server" {
			for _, authRef := range policy.Spec.RequiredAuthenticationRefs {
				if authRef.Kind == "MeshTLSAuthetication" && authRef.Name == v1beta1.ObjectName("meshtls-"+targetClient) {
					return false, nil
				}
			}
		}
	}
	return true, nil
}

func (ldm *LinkerdManager) generateLinkerdServer(
	intents otterizev1alpha3.ClientIntents,
	intent otterizev1alpha3.Intent,
	podSelector metav1.LabelSelector,
	port int32,
) *linkerdserver.Server {
	name := ldm.getServerName(intent, port)
	serverNamespace := intent.GetTargetServerNamespace(intents.Namespace)
	linkerdServerServiceFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)

	s := linkerdserver.Server{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1beta1",
			Kind:       "Server",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: serverNamespace,
			Labels: map[string]string{
				v1alpha3.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
			},
		},
		Spec: linkerdserver.ServerSpec{
			PodSelector: &podSelector,
			Port:        intstr.FromInt32(port),
		},
	}
	return &s
}

func (ldm *LinkerdManager) generateAuthorizationPolicy(
	intents otterizev1alpha3.ClientIntents,
	server,
	meshtls string,
) *authpolicy.AuthorizationPolicy {
	linkerdServerServiceFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)

	a := authpolicy.AuthorizationPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1alpha1",
			Kind:       "AuthorizationPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(OtterizeLinkerdAuthPolicyNameTemplate, server, meshtls),
			Namespace: intents.Namespace,
			Labels: map[string]string{
				v1alpha3.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
			},
		},
		Spec: authpolicy.AuthorizationPolicySpec{
			TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
				Group: "policy.linkerd.io",
				Kind:  "Server",
				Name:  v1beta1.ObjectName(server),
			},
			RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
				{
					Group: "policy.linkerd.io",
					Kind:  "MeshTLSAuthentication",
					Name:  v1beta1.ObjectName(meshtls),
				},
			},
		},
	}
	return &a
}

func (ldm *LinkerdManager) generateMeshTLS(
	intents otterizev1alpha3.ClientIntents,
	intent otterizev1alpha3.Intent,
	targets []string,
) *authpolicy.MeshTLSAuthentication {
	serverNamespace := intent.GetTargetServerNamespace(intents.Namespace)
	formattedMeshTLSTarget := v1alpha2.GetFormattedOtterizeIdentity(intent.GetTargetServerName(), serverNamespace)

	mtls := authpolicy.MeshTLSAuthentication{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1alpha1",
			Kind:       "MeshTLSAuthentication",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, formattedMeshTLSTarget),
			Namespace: intents.Namespace,
			Labels:    map[string]string{otterizev1alpha3.OtterizeLinkerdMeshTLSAnnotationKey: formattedMeshTLSTarget},
		},
		Spec: authpolicy.MeshTLSAuthenticationSpec{
			Identities: targets,
		},
	}
	return &mtls
}