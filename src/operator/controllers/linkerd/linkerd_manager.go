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
	corev1 "k8s.io/api/core/v1"
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
	OtterizeLinkerdServerNameTemplate     = "server-for-%s-port-%d"
	OtterizeLinkerdMeshTLSNameTemplate    = "meshtls-for-client-%s"
	OtterizeLinkerdAuthPolicyNameTemplate = "authorization-policy-to-%s-port-%d-from-client-%s"
	ReasonDeleteLinkerdPolicyFailed       = "DeleteLinkerdPolicyFailed"
	ReasonNamespaceNotAllowed             = "NamespaceNotAllowed"
	ReasonLinkerdPolicy                   = "LinkerdPolicy"
	ReasonMissingSidecar                  = "MissingSideCar"
	ReasonCreatingLinkerdPolicyFailed     = "CreatingLinkerdPolicyFailed"
	ReasonUpdatingLinkerdPolicyFailed     = "UpdatingLinkerdPolicyFailed"
	FullServiceAccountName                = "%s.%s.serviceaccount.identity.linkerd.cluster.local"
)

type LinkerdPolicyManager interface {
	DeleteAll(ctx context.Context, clientIntents *v1alpha3.ClientIntents) error
	Create(ctx context.Context, clientIntents *v1alpha3.ClientIntents, clientServiceAccount string) error
	UpdateIntentsStatus(ctx context.Context, clientIntents *v1alpha3.ClientIntents, clientServiceAccount string, missingSideCar bool) error
	UpdateServerSidecar(ctx context.Context, clientIntents *v1alpha3.ClientIntents, serverName string, missingSideCar bool) error
}

type LinkerdManager struct {
	client.Client
	serviceIdResolver           serviceidresolver.ServiceResolver
	recorder                    *injectablerecorder.InjectableRecorder
	restrictedToNamespaces      []string
	enforcementDefaultState     bool
	enableLinkerdPolicyCreation bool
}

func NewLinkerdManager(c client.Client,
	namespaces []string,
	r *injectablerecorder.InjectableRecorder,
	enforcementDefaultState,
	enableLinkerdPolicyCreation bool) *LinkerdManager {
	return &LinkerdManager{
		Client:                      c,
		serviceIdResolver:           serviceidresolver.NewResolver(c),
		restrictedToNamespaces:      namespaces,
		recorder:                    r,
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
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd policies: %s", err.Error())
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
		if intent.Type != "" && intent.Type != v1alpha3.IntentTypeHTTP && intent.Port != 0 { // this will skip non http ones, db for example, skip port doesnt exist as well
			continue
		}
		// if http intent logic
		shouldCreatePolicy, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState( //TODO:  check what that does
			ctx, ldm.Client, intent.GetTargetServerName(), intent.GetTargetServerNamespace(clientIntents.Namespace), ldm.enforcementDefaultState)
		if err != nil {
			return nil, err
		}

		if !shouldCreatePolicy {
			logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping network policy creation for server %s in namespace %s", intent.GetTargetServerName(), intent.GetTargetServerNamespace(clientIntents.Namespace))
			ldm.recorder.RecordNormalEventf(clientIntents, consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally and called service '%s' is not explicitly protected using a ProtectedService resource, network policy creation skipped", intent.Name)
			continue
		}

		if !ldm.enableLinkerdPolicyCreation {
			ldm.recorder.RecordNormalEvent(clientIntents, consts.ReasonIstioPolicyCreationDisabled, "Linkerd policy creation is disabled, creation skipped")
			return updatedPolicies, nil
		}

		targetNamespace := intent.GetTargetServerNamespace(clientIntents.Namespace)
		if len(ldm.restrictedToNamespaces) != 0 && !lo.Contains(ldm.restrictedToNamespaces, targetNamespace) {
			ldm.recorder.RecordWarningEventf(
				clientIntents,
				ReasonNamespaceNotAllowed,
				"Namespace %s was specified in intent, but is not allowed by configuration, Linkerd policy ignored",
				targetNamespace,
			)
			continue
		}

		// check if there's a server for that service
		s, shouldCreateServer, err := ldm.shouldCreateServer(ctx, *clientIntents, intent)
		if err != nil {
			return nil, err
		}

		if shouldCreateServer {
			podSelector := ldm.BuildPodLabelSelectorFromIntent(intent, clientIntents.Namespace)
			s = ldm.generateLinkerdServer(*clientIntents, intent, podSelector, intent.Port)
			err = ldm.Client.Create(ctx, s)
			if err != nil {
				ldm.recorder.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd server: %s", err.Error())
				return nil, err
			}
		}

		// if intent.Type == otterizev1alpha3.IntentTypeHTTP {
		/*
			1- parse the intent as linkerd http route resource

			2- check if its the first httproute wih this server as parent, if its not skip to 5

			3- if it is check if this deployment uses a httpget or tcp socket for probes

			4- if it does create a httproute and network authentication

			5- check if you should create the resouce
			if you should then create
		*/
		// }

		shouldCreateMeshTLS, err := ldm.shouldCreateMeshTLS(ctx, *clientIntents, clientIntents.Spec.Service.Name)
		if err != nil {
			return nil, err
		}
		fullServiceAccountName := fmt.Sprintf(FullServiceAccountName, clientServiceAccount, clientIntents.Namespace)

		if shouldCreateMeshTLS {
			mtls := ldm.generateMeshTLS(*clientIntents, intent, []string{fullServiceAccountName})
			err = ldm.Client.Create(ctx, mtls)
			if err != nil {
				ldm.recorder.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd meshTLS: %s", err.Error())
				return nil, err
			}
		}

		shouldCreatePolicy, err = ldm.shouldCreateAuthPolicy(ctx, *clientIntents, s.Name)
		if err != nil {
			return nil, err
		}

		if shouldCreatePolicy {
			newPolicy := ldm.generateAuthorizationPolicy(*clientIntents, intent, s.Name)
			err = ldm.Client.Create(ctx, newPolicy)
			if err != nil {
				ldm.recorder.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd policy: %s", err.Error())
				return nil, err
			}
			createdAnyPolicies = true
		}
	}

	if updatedPolicies.Len() != 0 || createdAnyPolicies { // TODO: understand this
		ldm.recorder.RecordNormalEventf(clientIntents, ReasonLinkerdPolicy, "Linkerd policy reconcile complete, reconciled %d servers", len(clientIntents.GetCallsList()))
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

func (ldm *LinkerdManager) otterizeHTTPIntentToLinkerd() {}

func (ldm *LinkerdManager) checkFirstHTTPRoute() {}

func (ldm *LinkerdManager) getLivenessProbePath(ctx context.Context, intents otterizev1alpha3.ClientIntents,
	intent otterizev1alpha3.Intent) (string, error) {
	pod, err := ldm.serviceIdResolver.ResolveIntentServerToPod(ctx, intent, intents.Namespace)
	if err != nil {
		return "", err
	}
	// should be the container that defines the port
	c := ldm.getContainerWithIntentPort(intent, &pod)
	if c == nil {
		return "", fmt.Errorf("no container in the pod specifies the intent port, skip HTTPRoute creation")
	}

	// TODO: check of the other probe types will break
	if c.LivenessProbe.HTTPGet != nil {
		return c.LivenessProbe.HTTPGet.Path, nil
	}
	return "", fmt.Errorf("probe path could not be found!, should skip HTTPRoute creation")
}

// kosom el fesa bsra7a
func pointershenanigans(ap authpolicy.PathMatchType) *authpolicy.PathMatchType {
	return &ap
}

func (ldm *LinkerdManager) generateHTTPProbeRoute(intent otterizev1alpha3.Intent, isProbeRoute bool,
	serverName, path, namespace string) *authpolicy.HTTPRoute {
	routeName := fmt.Sprintf("%s-probe-http-route-on-port-%s", intent.Name, intent.Port)
	return &authpolicy.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1beta1",
			Kind:       "Server",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: namespace,
		},
		Spec: authpolicy.HTTPRouteSpec{
			Rules: []authpolicy.HTTPRouteRule{
				{
					Matches: []authpolicy.HTTPRouteMatch{
						{
							Path: &authpolicy.HTTPPathMatch{
								Type:  pointershenanigans(authpolicy.PathMatchPathPrefix),
								Value: &path,
							},
						},
					},
				},
			},
		},
	}
}

func (ldm *LinkerdManager) generateNetworkAuthentication() {}

func (ldm *LinkerdManager) shouldCreateServer(ctx context.Context, intents otterizev1alpha3.ClientIntents, intent otterizev1alpha3.Intent) (*linkerdserver.Server, bool, error) {
	linkerdServerServiceFormattedIdentity := otterizev1alpha3.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)
	podSelector := ldm.BuildPodLabelSelectorFromIntent(intent, intents.Namespace)
	servers := &linkerdserver.ServerList{}
	err := ldm.Client.List(ctx, servers, client.MatchingLabels{v1alpha3.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity})
	if err != nil {
		return nil, false, err
	}

	// no servers exist
	if len(servers.Items) == 0 {
		return nil, true, nil
	}

	// get servers in the namespace and if any of them has a label selector similar to the intents label return that server
	for _, server := range servers.Items {
		if intent.Port == server.Spec.Port.IntVal && server.Spec.PodSelector == &podSelector {
			return &server, false, nil
		}
	}
	return nil, true, nil
}

func (ldm *LinkerdManager) shouldCreateMeshTLS(ctx context.Context, intents otterizev1alpha3.ClientIntents, clientName string) (bool, error) {
	// network authentication ?
	meshes := &authpolicy.MeshTLSAuthenticationList{}

	err := ldm.Client.List(ctx, meshes, &client.ListOptions{Namespace: intents.Namespace})
	if err != nil {
		return false, err
	}
	for _, mesh := range meshes.Items {
		if mesh.Name == fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, clientName) {
			return false, nil
		}
		/*
			create a meshtls for the client if a meshtls with the same name doesnt exist,
			even if the clientsa is authorized in different meshtls this is to not authorize other identities that are not needed
			these other identities can have their own auth policies later
		*/
	}
	return true, nil
}

func (ldm *LinkerdManager) shouldCreateAuthPolicy(ctx context.Context, intents otterizev1alpha3.ClientIntents, targetServer string) (bool, error) {
	/*
		this should say an auth policy should be created if there doesnt exist an auth policy that targets the server
		in question and in its required auth ref is a meshtls with name as meshtls client
	*/
	logrus.Infof("checking if i should create an authpolicy for %s and %s", targetServer, intents.Spec.Service.Name)
	authPolicies := &authpolicy.AuthorizationPolicyList{}

	err := ldm.Client.List(ctx, authPolicies, &client.ListOptions{Namespace: intents.Namespace}) // check if auth policies can work across namespaces, in this case this wont work
	if err != nil {
		return false, err
	}
	for _, policy := range authPolicies.Items {
		if policy.Spec.TargetRef.Name == v1beta1.ObjectName(targetServer) && policy.Spec.TargetRef.Kind == "Server" {
			for _, authRef := range policy.Spec.RequiredAuthenticationRefs {
				if authRef.Kind == "MeshTLSAuthetication" && authRef.Name == v1beta1.ObjectName("meshtls-for-client-"+intents.Spec.Service.Name) {
					logrus.Infof("not creating policy for policy with details, %s, %s", policy.Spec.TargetRef.Name, authRef.Name)
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
	logrus.Infof("Generating server with details: %+v, %+v, %d", linkerdServerServiceFormattedIdentity, serverNamespace, port)

	s := linkerdserver.Server{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1beta1",
			Kind:       "Server",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: serverNamespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
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
	intent otterizev1alpha3.Intent,
	serverTargetName string,
) *authpolicy.AuthorizationPolicy {
	a := authpolicy.AuthorizationPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1alpha1",
			Kind:       "AuthorizationPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(OtterizeLinkerdAuthPolicyNameTemplate, intent.Name, intent.Port, intents.Spec.Service.Name),
			Namespace: intents.Namespace,
		},
		Spec: authpolicy.AuthorizationPolicySpec{
			TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
				Group: "policy.linkerd.io",
				Kind:  "Server",
				Name:  v1beta1.ObjectName(serverTargetName),
			},
			RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
				{
					Group: "policy.linkerd.io",
					Kind:  "MeshTLSAuthentication",
					Name:  v1beta1.ObjectName(fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, intents.Spec.Service.Name)),
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
	mtls := authpolicy.MeshTLSAuthentication{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1alpha1",
			Kind:       "MeshTLSAuthentication",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, intents.Spec.Service.Name),
			Namespace: intents.Namespace,
		},
		Spec: authpolicy.MeshTLSAuthenticationSpec{
			Identities: targets,
		},
	}
	return &mtls
}

func (ldm *LinkerdManager) getContainerWithIntentPort(intent otterizev1alpha3.Intent, pod *corev1.Pod) *corev1.Container {
	for _, container := range pod.Spec.Containers {
		for _, port := range container.Ports {
			if port.ContainerPort == intent.Port {
				return &container
			}
		}
	}
	return nil
}
