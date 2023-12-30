package linkerdmanager

import (
	"context"
	"fmt"
	"strings"

	"github.com/amit7itz/goset"
	authpolicy "github.com/linkerd/linkerd2/controller/gen/apis/policy/v1alpha1"
	linkerdserver "github.com/linkerd/linkerd2/controller/gen/apis/server/v1beta1"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
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

// TODO: make sure all generated resources are tagged with the intent otterize identity

const (
	ReasonGettingLinkerdPolicyFailed                  = "GettingLinkerdPolicyFailed"
	OtterizeLinkerdServerNameTemplate                 = "server-for-%s-port-%d"
	OtterizeLinkerdMeshTLSNameTemplate                = "meshtls-for-client-%s"
	OtterizeLinkerdAuthPolicyNameTemplate             = "authorization-policy-to-%s-port-%d-from-client-%s"
	OtterizeLinkerdAuthPolicyForHTTPRouteNameTemplate = "authorization-policy-to-%s-port-%d-from-client-%s-path-%s"
	ReasonDeleteLinkerdPolicyFailed                   = "DeleteLinkerdPolicyFailed"
	ReasonNamespaceNotAllowed                         = "NamespaceNotAllowed"
	ReasonLinkerdPolicy                               = "LinkerdPolicy"
	ReasonMissingSidecar                              = "MissingSideCar"
	ReasonCreatingLinkerdPolicyFailed                 = "CreatingLinkerdPolicyFailed"
	ReasonUpdatingLinkerdPolicyFailed                 = "UpdatingLinkerdPolicyFailed"
	FullServiceAccountName                            = "%s.%s.serviceaccount.identity.linkerd.cluster.local"
	NetworkAuthenticationNameTemplate                 = "network-auth-for-probe-routes"
	HTTPRouteNameTemplate                             = "http-route-for-%s-port-%d-path-%s"
	LinkerdMeshTLSAuthenticationKindName              = "MeshTLSAuthentication"
	LinkerdServerKindName                             = "Server"
	LinkerdHTTPRouteKindName                          = "HTTPRoute"
	LinkerdNetAuthKindName                            = "NetworkAuthentication"
	Servers                                           = "servers"
	Routes                                            = "httproutes"
	AuthorizationPolicies                             = "authpolicies"
	MTLSAuthentications                               = "mtlsauth"
	NetworkAuthentications                            = "netauth"
)

type LinkerdPolicyManager interface {
	DeleteAll(ctx context.Context, clientIntents *otterizev1alpha3.ClientIntents) error
	Create(ctx context.Context, clientIntents *otterizev1alpha3.ClientIntents, clientServiceAccount string) error
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
	clientIntents *otterizev1alpha3.ClientIntents,
	clientServiceAccount string,
) error {
	clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(clientIntents.Spec.Service.Name, clientIntents.Namespace)

	var (
		existingPolicies   authpolicy.AuthorizationPolicyList
		existingServers    linkerdserver.ServerList
		existingHttpRoutes authpolicy.HTTPRouteList
	)
	// TODO: the struct method works here
	err := ldm.Client.List(ctx,
		&existingPolicies,
		client.MatchingLabels{otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd policies: %s", err.Error())
		return err
	}

	err = ldm.Client.List(ctx,
		&existingServers,
		client.MatchingLabels{otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd servers: %s", err.Error())
		return err
	}

	err = ldm.Client.List(ctx,
		&existingHttpRoutes,
		client.MatchingLabels{otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd http routes: %s", err.Error())
		return err
	}

	validResources, err := ldm.createResources(ctx, clientIntents, clientServiceAccount)
	if err != nil {
		return err
	}

	err = ldm.deleteOutdatedResources(ctx, validResources,
		existingPolicies,
		existingServers,
		existingHttpRoutes,
	)
	if err != nil {
		return err
	}
	return nil
}

func (ldm *LinkerdManager) DeleteAll(ctx context.Context,
	intents *otterizev1alpha3.ClientIntents) error {
	clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(intents.Spec.Service.Name, intents.Namespace)

	var (
		existingPolicies   authpolicy.AuthorizationPolicyList
		existingServers    linkerdserver.ServerList
		existingHttpRoutes authpolicy.HTTPRouteList
	)
	// TODO: the struct method works here
	// TODO: netauth and meshtls
	err := ldm.Client.List(ctx,
		&existingPolicies,
		client.MatchingLabels{otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(intents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd policies: %s", err.Error())
		return err
	}

	err = ldm.Client.List(ctx,
		&existingServers,
		client.MatchingLabels{otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(intents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd servers: %s", err.Error())
		return err
	}

	err = ldm.Client.List(ctx,
		&existingHttpRoutes,
		client.MatchingLabels{otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(intents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd http routes: %s", err.Error())
		return err
	}

	for _, existingPolicy := range existingPolicies.Items {
		err := ldm.Client.Delete(ctx, &existingPolicy)
		if err != nil {
			return err
		}
	}

	for _, existingServer := range existingServers.Items {
		err := ldm.Client.Delete(ctx, &existingServer)
		if err != nil {
			return err
		}
	}

	for _, existingRoute := range existingHttpRoutes.Items {
		err := ldm.Client.Delete(ctx, &existingRoute)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ldm *LinkerdManager) deleteOutdatedResources(ctx context.Context,
	validResources map[string]*goset.Set[types.UID],
	existingPolicies authpolicy.AuthorizationPolicyList,
	existingServers linkerdserver.ServerList,
	existingRoutes authpolicy.HTTPRouteList) error {
	for _, existingPolicy := range existingPolicies.Items {
		if !validResources[AuthorizationPolicies].Contains(existingPolicy.UID) {
			err := ldm.Client.Delete(ctx, &existingPolicy)
			if err != nil {
				return err
			}
		}
	}

	for _, existingServer := range existingServers.Items {
		if !validResources[Servers].Contains(existingServer.UID) {
			err := ldm.Client.Delete(ctx, &existingServer)
			if err != nil {
				return err
			}
		}
	}

	for _, existingRoute := range existingRoutes.Items {
		if !validResources[Routes].Contains(existingRoute.UID) {
			err := ldm.Client.Delete(ctx, &existingRoute)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (ldm *LinkerdManager) createResources(
	ctx context.Context,
	clientIntents *otterizev1alpha3.ClientIntents,
	clientServiceAccount string, // supplied in the create method
) (map[string]*goset.Set[types.UID], error) {
	currentResources := map[string]*goset.Set[types.UID]{
		Servers:               goset.NewSet[types.UID](),
		AuthorizationPolicies: goset.NewSet[types.UID](),
		Routes:                goset.NewSet[types.UID](),
	}

	for _, intent := range clientIntents.GetCallsList() {
		if intent.Type != "" && intent.Type != otterizev1alpha3.IntentTypeHTTP && intent.Port != 0 { // this will skip non http ones, db for example, skip port doesnt exist as well
			continue
		}
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
			return nil, nil
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
		err = ldm.createIntentPrimaryResources(ctx, *clientIntents, clientServiceAccount) // TODO: move to reconciler
		if err != nil {
			return nil, err
		}

		// check if there's a server for that service
		s, shouldCreateServer, err := ldm.shouldCreateServer(ctx, *clientIntents, intent)
		if err != nil {
			return nil, err
		}
		logrus.Info("shoudl create server result", shouldCreateServer)

		if shouldCreateServer {
			podSelector := ldm.BuildPodLabelSelectorFromIntent(intent, clientIntents.Namespace)
			s = ldm.generateLinkerdServer(*clientIntents, intent, podSelector)
			err = ldm.Client.Create(ctx, s)
			if err != nil {
				ldm.recorder.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd server: %s", err.Error())
				return nil, err
			}
		}
		currentResources[Servers].Add(s.UID)

		switch intent.Type {
		case otterizev1alpha3.IntentTypeHTTP:
			serverHasHTTPRoute, err := ldm.doesServerHaveHTTPRoute(ctx, *clientIntents, s.Name)
			if err != nil {
				return nil, err
			}
			logrus.Info("does server %s have a http route ? %s", s.Name, serverHasHTTPRoute)

			probePath, err := ldm.getLivenessProbePath(ctx, *clientIntents, intent)
			if err != nil {
				return nil, err
			}
			logrus.Info("probe path: ", probePath)

			if !serverHasHTTPRoute && probePath != "" {
				httpRouteName := fmt.Sprintf(HTTPRouteNameTemplate, intent.Name, intent.Port, probePath)
				httpRouteName = strings.Replace(httpRouteName, "/", "slash", -1)
				probePathRoute := ldm.generateHTTPRoute(*clientIntents, intent, s.Name, probePath, httpRouteName, clientIntents.Namespace)
				logrus.Info("probe path name: ", probePathRoute.Name)
				err = ldm.Client.Create(ctx, probePathRoute)
				if err != nil {
					return nil, err
				}

				authPolicy := ldm.generateAuthorizationPolicy(*clientIntents, intent, httpRouteName,
					LinkerdHTTPRouteKindName,
					LinkerdNetAuthKindName,
					addPath(probePath))
				logrus.Info("policyname: ", authPolicy.Name)

				err = ldm.Client.Create(ctx, authPolicy)
				if err != nil {
					return nil, err
				}
				currentResources[Routes].Add(probePathRoute.UID)
				currentResources[AuthorizationPolicies].Add(authPolicy.UID)
				logrus.Info("processed probe route")
			}

			for _, httpResource := range intent.HTTPResources {
				httpRouteName := fmt.Sprintf(HTTPRouteNameTemplate, intent.Name, intent.Port, httpResource.Path)
				httpRouteName = strings.Replace(httpRouteName, "/", "slash", -1)
				route, shouldCreateRoute, err := ldm.shouldCreateHTTPRoute(ctx, *clientIntents,
					intent, httpRouteName)
				if err != nil {
					return nil, err
				}

				if shouldCreateRoute {
					route = ldm.generateHTTPRoute(*clientIntents, intent, s.Name, httpResource.Path,
						httpRouteName,
						clientIntents.Namespace)
					logrus.Info("route name: ", route.Name)
					err = ldm.Client.Create(ctx, route)
					if err != nil { // TODO: return errors but continue processing
						return nil, err
					}
				}
				currentResources[Routes].Add(route.UID)
				// should create authpolicy
				policy, shouldCreatePolicy, err := ldm.shouldCreateAuthPolicy(ctx,
					*clientIntents, route.Name,
					LinkerdHTTPRouteKindName,
					LinkerdMeshTLSAuthenticationKindName)
				if err != nil {
					return nil, err
				}

				if shouldCreatePolicy {
					policy = ldm.generateAuthorizationPolicy(*clientIntents, intent, s.Name,
						LinkerdHTTPRouteKindName,
						LinkerdMeshTLSAuthenticationKindName,
						addPath(httpResource.Path))
					err = ldm.Client.Create(ctx, policy)
					if err != nil {
						ldm.recorder.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd policy: %s", err.Error())
						return nil, err
					}
				}
				currentResources[AuthorizationPolicies].Add(policy.UID)
			}
		default:
			policy, shouldCreatePolicy, err := ldm.shouldCreateAuthPolicy(ctx, *clientIntents,
				s.Name,
				LinkerdServerKindName,
				LinkerdMeshTLSAuthenticationKindName)
			if err != nil {
				return nil, err
			}

			if shouldCreatePolicy {
				policy = ldm.generateAuthorizationPolicy(*clientIntents, intent, s.Name,
					LinkerdServerKindName,
					LinkerdMeshTLSAuthenticationKindName)
				err = ldm.Client.Create(ctx, policy)
				if err != nil {
					ldm.recorder.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd policy: %s", err.Error())
					return nil, err
				}
			}
			currentResources[AuthorizationPolicies].Add(policy.UID)
		}
	}
	return currentResources, nil
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

func (ldm *LinkerdManager) createIntentPrimaryResources(ctx context.Context,
	intents otterizev1alpha3.ClientIntents,
	clientServiceAccount string) error {
	shouldCreateNetAuth, err := ldm.shouldCreateNetAuth(ctx, intents)
	if err != nil {
		return err
	}

	if shouldCreateNetAuth {
		netAuth := ldm.generateNetworkAuthentication(intents)
		err := ldm.Client.Create(ctx, netAuth)
		if err != nil {
			return err
		}
	}

	shouldCreateMeshTLS, err := ldm.shouldCreateMeshTLS(ctx, intents)
	if err != nil {
		return err
	}
	fullServiceAccountName := fmt.Sprintf(FullServiceAccountName, clientServiceAccount, intents.Namespace)

	if shouldCreateMeshTLS {
		mtls := ldm.generateMeshTLS(intents, []string{fullServiceAccountName})
		err = ldm.Client.Create(ctx, mtls)
		if err != nil {
			ldm.recorder.RecordWarningEventf(&intents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd meshTLS: %s", err.Error())
			return err
		}
	}
	return nil
}

func (ldm *LinkerdManager) doesServerHaveHTTPRoute(ctx context.Context, intents otterizev1alpha3.ClientIntents, serverName string) (bool, error) {
	httpRoutes := &authpolicy.HTTPRouteList{}
	err := ldm.Client.List(ctx, httpRoutes, &client.ListOptions{Namespace: intents.Namespace})
	if err != nil {
		return false, err
	}
	for _, route := range httpRoutes.Items {
		for _, parent := range route.Spec.ParentRefs {
			// check for potential nil pointer derefrence
			if *parent.Kind == "Server" && parent.Name == v1beta1.ObjectName(serverName) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (ldm *LinkerdManager) getLivenessProbePath(ctx context.Context, intents otterizev1alpha3.ClientIntents,
	intent otterizev1alpha3.Intent) (string, error) {
	pod, err := ldm.serviceIdResolver.ResolveIntentServerToPod(ctx, intent, intents.Namespace)
	if err != nil {
		return "", err
	}
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

func getPathMatchPointer(ap authpolicy.PathMatchType) *authpolicy.PathMatchType {
	return &ap
}

func (ldm *LinkerdManager) shouldCreateServer(ctx context.Context, intents otterizev1alpha3.ClientIntents, intent otterizev1alpha3.Intent) (*linkerdserver.Server, bool, error) {
	linkerdServerServiceFormattedIdentity := otterizev1alpha3.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)
	podSelector := ldm.BuildPodLabelSelectorFromIntent(intent, intents.Namespace)
	servers := &linkerdserver.ServerList{}
	logrus.Infof("shoudl create server ? %s", podSelector.String())
	err := ldm.Client.List(ctx, servers, client.MatchingLabels{otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity})
	if err != nil {
		return nil, false, err
	}

	// no servers exist
	if len(servers.Items) == 0 {
		logrus.Info("found no servers")
		return nil, true, nil
	}

	// get servers in the namespace and if any of them has a label selector similar to the intents label return that server
	for _, server := range servers.Items {
		if server.Name == fmt.Sprintf(OtterizeLinkerdServerNameTemplate, intent.Name, intent.Port) {
			return &server, false, nil
		}
	}
	return nil, true, nil
}

func (ldm *LinkerdManager) shouldCreateHTTPRoute(ctx context.Context,
	intents otterizev1alpha3.ClientIntents,
	intent otterizev1alpha3.Intent,
	httpRouteName string,
) (*authpolicy.HTTPRoute, bool, error) {
	routes := &authpolicy.HTTPRouteList{}
	err := ldm.Client.List(ctx, routes, &client.ListOptions{Namespace: intents.Namespace})
	if err != nil {
		return nil, false, err
	}
	for _, route := range routes.Items {
		if route.Name == httpRouteName { // TODO: validate this better, just the name isnt enough
			return &route, false, nil
		}
	}
	return nil, true, nil
}

func (ldm *LinkerdManager) shouldCreateAuthPolicy(ctx context.Context,
	intents otterizev1alpha3.ClientIntents,
	targetName,
	targetRefKind,
	authRefKind string) (*authpolicy.AuthorizationPolicy, bool, error) {
	logrus.Infof("checking if i should create an authpolicy for %s and %s", targetName, intents.Spec.Service.Name)
	authPolicies := &authpolicy.AuthorizationPolicyList{}

	err := ldm.Client.List(ctx, authPolicies, &client.ListOptions{Namespace: intents.Namespace}) // check if auth policies can work across namespaces, in this case this wont work
	if err != nil {
		return nil, false, err
	}
	for _, policy := range authPolicies.Items {
		if policy.Spec.TargetRef.Name == v1beta1.ObjectName(targetName) && policy.Spec.TargetRef.Kind == v1beta1.Kind(targetRefKind) {
			for _, authRef := range policy.Spec.RequiredAuthenticationRefs {
				if authRef.Kind == v1beta1.Kind(authRefKind) && authRef.Name == v1beta1.ObjectName("meshtls-for-client-"+intents.Spec.Service.Name) { // TODO: check for authrefname too
					logrus.Infof("not creating policy for policy with details, %s, %s", policy.Spec.TargetRef.Name, authRef.Name)
					return &policy, false, nil
				}
			}
		}
	}
	return nil, true, nil
}

func (ldm *LinkerdManager) shouldCreateMeshTLS(ctx context.Context, intents otterizev1alpha3.ClientIntents) (bool, error) {
	meshes := &authpolicy.MeshTLSAuthenticationList{}
	err := ldm.Client.List(ctx, meshes, &client.ListOptions{Namespace: intents.Namespace})
	if err != nil {
		return false, err
	}
	for _, mesh := range meshes.Items {
		if mesh.Name == fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, intents.Spec.Service.Name) {
			return false, nil
		}
	}
	return true, nil
}

func (ldm *LinkerdManager) shouldCreateNetAuth(ctx context.Context, intents otterizev1alpha3.ClientIntents) (bool, error) {
	netauths := &authpolicy.NetworkAuthenticationList{}
	err := ldm.Client.List(ctx, netauths, &client.ListOptions{Namespace: intents.Namespace})
	if err != nil {
		return false, err
	}
	for _, netauth := range netauths.Items {
		if netauth.Name == NetworkAuthenticationNameTemplate {
			return false, nil
		}
	}
	return true, nil
}

func (ldm *LinkerdManager) generateLinkerdServer(
	intents otterizev1alpha3.ClientIntents,
	intent otterizev1alpha3.Intent,
	podSelector metav1.LabelSelector,
) *linkerdserver.Server {
	name := ldm.getServerName(intent, intent.Port)
	serverNamespace := intent.GetTargetServerNamespace(intents.Namespace)
	linkerdServerServiceFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)
	logrus.Infof("Generating server with details: %+v, %+v, %d", linkerdServerServiceFormattedIdentity, serverNamespace, intent.Port)

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
			Port:        intstr.FromInt32(intent.Port),
		},
	}
	return &s
}

type policyOpts func(*authpolicy.AuthorizationPolicy)

func addPath(path string) policyOpts {
	return func(policy *authpolicy.AuthorizationPolicy) {
		replacedString := strings.Replace(path, "/", "slash", -1)
		policy.Name = policy.Name + "-path-" + replacedString
		logrus.Info("policy name in the optional func: ", policy.Name)
	}
}

func (ldm *LinkerdManager) generateAuthorizationPolicy(
	intents otterizev1alpha3.ClientIntents,
	intent otterizev1alpha3.Intent,
	serverTargetName,
	targetRefType,
	requiredAuthRefType string,
	authPolicyOpts ...policyOpts,
) *authpolicy.AuthorizationPolicy {
	var (
		targetRefName v1beta1.ObjectName
	)
	switch requiredAuthRefType {
	case LinkerdNetAuthKindName:
		targetRefName = v1beta1.ObjectName(NetworkAuthenticationNameTemplate)
	case LinkerdMeshTLSAuthenticationKindName:
		targetRefName = v1beta1.ObjectName(fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, intents.Spec.Service.Name))
	}
	linkerdServerServiceFormattedIdentity := otterizev1alpha3.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)
	a := authpolicy.AuthorizationPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1alpha1",
			Kind:       "AuthorizationPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(OtterizeLinkerdAuthPolicyNameTemplate, intent.Name, intent.Port, intents.Spec.Service.Name),
			Namespace: intents.Namespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
			},
		},
		Spec: authpolicy.AuthorizationPolicySpec{
			TargetRef: gatewayapiv1alpha2.PolicyTargetReference{
				Group: "policy.linkerd.io",
				Kind:  v1beta1.Kind(targetRefType),
				Name:  v1beta1.ObjectName(serverTargetName),
			},
			RequiredAuthenticationRefs: []gatewayapiv1alpha2.PolicyTargetReference{
				{
					Group: "policy.linkerd.io",
					Kind:  v1beta1.Kind(requiredAuthRefType),
					Name:  targetRefName,
				},
			},
		},
	}
	return &a
}

func StringPtr(s string) *string {
	return &s
}

func (ldm *LinkerdManager) generateHTTPRoute(intents otterizev1alpha3.ClientIntents, intent otterizev1alpha3.Intent,
	serverName, path, name, namespace string) *authpolicy.HTTPRoute { // TODO: dont take name as an argument
	linkerdServerServiceFormattedIdentity := otterizev1alpha3.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)
	return &authpolicy.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1beta3",
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
			},
		},
		Spec: authpolicy.HTTPRouteSpec{
			CommonRouteSpec: v1beta1.CommonRouteSpec{
				ParentRefs: []v1beta1.ParentReference{
					{
						Group: (*v1beta1.Group)(StringPtr("policy.linkerd.io")),
						Kind:  (*v1beta1.Kind)(StringPtr("Server")),
						Name:  v1beta1.ObjectName(serverName),
					},
				},
			},
			Rules: []authpolicy.HTTPRouteRule{

				{
					Matches: []authpolicy.HTTPRouteMatch{
						{
							Path: &authpolicy.HTTPPathMatch{
								Type:  getPathMatchPointer(authpolicy.PathMatchPathPrefix),
								Value: &path,
							},
							// Method: getHTTPMethodPointer(authpolicy.HTTPMethodGet), add support for that later
						},
					},
				},
			},
		},
	}
}

func (ldm *LinkerdManager) generateMeshTLS(
	intents otterizev1alpha3.ClientIntents,
	targets []string,
) *authpolicy.MeshTLSAuthentication {
	linkerdServerServiceFormattedIdentity := otterizev1alpha3.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)
	mtls := authpolicy.MeshTLSAuthentication{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1alpha1",
			Kind:       "MeshTLSAuthentication",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, intents.Spec.Service.Name),
			Namespace: intents.Namespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
			},
		},
		Spec: authpolicy.MeshTLSAuthenticationSpec{
			Identities: targets,
		},
	}
	return &mtls
}

func (ldm *LinkerdManager) generateNetworkAuthentication(intents otterizev1alpha3.ClientIntents) *authpolicy.NetworkAuthentication {
	linkerdServerServiceFormattedIdentity := otterizev1alpha3.GetFormattedOtterizeIdentity(intents.GetServiceName(), intents.Namespace)
	return &authpolicy.NetworkAuthentication{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1alpha1",
			Kind:       "NetworkAuthentication",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      NetworkAuthenticationNameTemplate,
			Namespace: intents.Namespace,
			Labels: map[string]string{
				otterizev1alpha3.OtterizeLinkerdServerAnnotationKey: linkerdServerServiceFormattedIdentity,
			},
		},
		Spec: authpolicy.NetworkAuthenticationSpec{
			Networks: []*authpolicy.Network{
				{
					Cidr: "0.0.0.0/0",
				},
				{
					Cidr: "::0",
				},
			},
		},
	}
}
