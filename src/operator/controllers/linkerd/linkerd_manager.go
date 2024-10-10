package linkerdmanager

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/otterize/intents-operator/src/shared/errors"

	authpolicy "github.com/linkerd/linkerd2/controller/gen/apis/policy/v1alpha1"
	linkerdserver "github.com/linkerd/linkerd2/controller/gen/apis/server/v1beta1"

	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/consts"
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

const (
	ReasonGettingLinkerdPolicyFailed                  = "GettingLinkerdPolicyFailed"
	OtterizeLinkerdServerNameTemplate                 = "server-for-%s-port-%d"
	OtterizeLinkerdMeshTLSNameTemplate                = "meshtls-for-client-%s"
	OtterizeLinkerdAuthPolicyNameTemplate             = "authpolicy-to-%s-port-%d-from-client-%s-%s"
	OtterizeLinkerdAuthPolicyProbeRouteNameTemplate   = "authpolicy-to-%s-port-%d-for-probe-path"
	OtterizeLinkerdAuthPolicyForHTTPRouteNameTemplate = "authorization-policy-to-%s-port-%d-from-client-%s-path-%s"
	ReasonDeleteLinkerdPolicyFailed                   = "DeleteLinkerdPolicyFailed"
	ReasonNamespaceNotAllowed                         = "NamespaceNotAllowed"
	ReasonLinkerdPolicy                               = "LinkerdPolicy"
	ReasonMissingSidecar                              = "MissingSideCar"
	ReasonCreatingLinkerdPolicyFailed                 = "CreatingLinkerdPolicyFailed"
	ReasonUpdatingLinkerdPolicyFailed                 = "UpdatingLinkerdPolicyFailed"
	FullServiceAccountName                            = "%s.%s.serviceaccount.identity.linkerd.cluster.local"
	NetworkAuthenticationNameTemplate                 = "network-auth-for-probe-routes"
	HTTPRouteNameTemplate                             = "http-route-for-%s-port-%d-%s"
	LinkerdMeshTLSAuthenticationKindName              = "MeshTLSAuthentication"
	LinkerdServerKindName                             = "Server"
	LinkerdHTTPRouteKindName                          = "HTTPRoute"
	LinkerdNetAuthKindName                            = "NetworkAuthentication"
	LinkerdContainer                                  = "linkerd-proxy"
)

//+kubebuilder:rbac:groups="policy.linkerd.io",resources=*,verbs=get;update;patch;list;watch;delete;create;deletecollection
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents,verbs=get;list;watch;create;update;patch;delete

type LinkerdPolicyManager interface {
	DeleteAll(ctx context.Context, clientIntents *otterizev2alpha1.ClientIntents) error
	Create(ctx context.Context, clientIntents *otterizev2alpha1.ClientIntents, clientServiceAccount string) error
}

type LinkerdResourceMapping struct {
	Servers               *goset.Set[types.UID]
	AuthorizationPolicies *goset.Set[types.UID]
	Routes                *goset.Set[types.UID]
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
	clientIntents *otterizev2alpha1.ClientIntents,
	clientServiceAccount string,
) error {

	clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(clientIntents.GetWorkloadName(), clientIntents.Namespace)

	var (
		existingPolicies   authpolicy.AuthorizationPolicyList
		existingServers    linkerdserver.ServerList
		existingHttpRoutes authpolicy.HTTPRouteList
	)
	// TODO: the struct method works here
	err := ldm.Client.List(ctx,
		&existingPolicies,
		client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd policies: %s", err.Error())
		return err
	}

	err = ldm.Client.List(ctx,
		&existingServers,
		client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd servers: %s", err.Error())
		return err
	}

	err = ldm.Client.List(ctx,
		&existingHttpRoutes,
		client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
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
	clientIntents *otterizev2alpha1.ClientIntents) error {
	clientFormattedIdentity := v1alpha2.GetFormattedOtterizeIdentity(clientIntents.GetWorkloadName(), clientIntents.Namespace)

	var (
		existingPolicies   authpolicy.AuthorizationPolicyList
		existingServers    linkerdserver.ServerList
		existingHttpRoutes authpolicy.HTTPRouteList
		existingNetAuth    authpolicy.NetworkAuthenticationList
		existingMTLS       authpolicy.MeshTLSAuthenticationList
		otherIntents       otterizev2alpha1.ClientIntentsList
	)

	err := ldm.Client.List(ctx,
		&existingPolicies,
		client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd policies: %s", err.Error())
		return err
	}

	err = ldm.Client.List(ctx,
		&existingServers,
		client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd servers: %s", err.Error())
		return err
	}

	err = ldm.Client.List(ctx,
		&existingHttpRoutes,
		client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd http routes: %s", err.Error())
		return err
	}

	err = ldm.Client.List(ctx,
		&existingNetAuth,
		client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd net auth: %s", err.Error())
		return err
	}

	err = ldm.Client.List(ctx,
		&existingMTLS,
		client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
	if err != nil {
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get Linkerd meshtls auth: %s", err.Error())
		return err
	}

	err = ldm.Client.List(ctx,
		&otherIntents,
		&client.ListOptions{Namespace: clientIntents.Namespace},
	)
	if err != nil {
		ldm.recorder.RecordWarningEventf(clientIntents, ReasonGettingLinkerdPolicyFailed, "Could not get client intents for recreation: %s", err.Error())
		return err
	}

	for _, existingPolicy := range existingPolicies.Items {
		err := ldm.Client.Delete(ctx, &existingPolicy)
		if err != nil {
			return err
		}
	}

	for _, server := range existingServers.Items {
		err := ldm.Client.Delete(ctx, &server)
		if err != nil {
			return err
		}
	}

	for _, route := range existingHttpRoutes.Items {
		err := ldm.Client.Delete(ctx, &route)
		if err != nil {
			return err
		}
	}

	for _, netAuth := range existingNetAuth.Items {
		err := ldm.Client.Delete(ctx, &netAuth)
		if err != nil {
			return err
		}
	}

	for _, mtlsAuth := range existingMTLS.Items {
		err := ldm.Client.Delete(ctx, &mtlsAuth)
		if err != nil {
			return err
		}
	}

	// recall create for other intents if resources belonging to other intents were deleted
	for _, otherIntent := range otherIntents.Items {
		if otherIntent.Name != clientIntents.Name {
			pod, err := ldm.serviceIdResolver.ResolveClientIntentToPod(ctx, otherIntent)
			if err != nil {
				return err
			}
			clientServiceAccountName := pod.Spec.ServiceAccountName
			missingSideCar := !IsPodPartOfLinkerdMesh(pod)

			if missingSideCar {
				logrus.Infof("Pod %s/%s does not have a sidecar, skipping Linkerd resource creation", pod.Namespace, pod.Name)
				return err
			}
			err = ldm.Create(ctx, &otherIntent, clientServiceAccountName)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (ldm *LinkerdManager) deleteOutdatedResources(ctx context.Context,
	validResources *LinkerdResourceMapping,
	existingPolicies authpolicy.AuthorizationPolicyList,
	existingServers linkerdserver.ServerList,
	existingRoutes authpolicy.HTTPRouteList) error {
	for _, existingPolicy := range existingPolicies.Items {
		if !validResources.AuthorizationPolicies.Contains(existingPolicy.UID) {
			err := ldm.Client.Delete(ctx, &existingPolicy)
			if err != nil {
				return err
			}
		}
	}

	for _, existingServer := range existingServers.Items {
		if !validResources.Servers.Contains(existingServer.UID) {
			err := ldm.Client.Delete(ctx, &existingServer)
			if err != nil {
				return err
			}
		}
	}

	for _, existingRoute := range existingRoutes.Items {
		if !validResources.Routes.Contains(existingRoute.UID) {
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
	clientIntents *otterizev2alpha1.ClientIntents,
	clientServiceAccount string,
) (*LinkerdResourceMapping, error) {
	currentResources := &LinkerdResourceMapping{
		Servers:               goset.NewSet[types.UID](),
		AuthorizationPolicies: goset.NewSet[types.UID](),
		Routes:                goset.NewSet[types.UID](),
	}

	for _, target := range clientIntents.GetTargetList() {
		if !target.IsTargetInCluster() { // this will skip non http ones, db for example, skip port doesnt exist as well
			continue
		}

		//shouldCreateLinkerdResources, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(
		//	ctx, ldm.Client, intent.GetS, intent.GetTargetServerNamespace(clientIntents.Namespace), ldm.enforcementDefaultState)
		//if err != nil {
		//	return nil, err
		//}

		if !true {
			logrus.Infof("Enforcement is disabled globally and server is not explicitly protected, skipping linkerd policy creation for server %s in namespace %s", target.GetTargetServerName(), target.GetTargetServerNamespace(clientIntents.Namespace))
			ldm.recorder.RecordNormalEventf(clientIntents, consts.ReasonEnforcementDefaultOff, "Enforcement is disabled globally and called service '%s' is not explicitly protected using a ProtectedService resource, linkerd policy creation skipped", target.GetTargetServerName())
			continue
		}

		if !ldm.enableLinkerdPolicyCreation {
			ldm.recorder.RecordNormalEvent(clientIntents, consts.ReasonIstioPolicyCreationDisabled, "Linkerd policy creation is disabled, creation skipped")
			return nil, nil
		}

		targetNamespace := target.GetTargetServerNamespace(clientIntents.Namespace)
		if len(ldm.restrictedToNamespaces) != 0 && !lo.Contains(ldm.restrictedToNamespaces, targetNamespace) {
			ldm.recorder.RecordWarningEventf(
				clientIntents,
				ReasonNamespaceNotAllowed,
				"Namespace %s was specified in intent, but is not allowed by configuration, Linkerd policy ignored",
				targetNamespace,
			)
			continue
		}

		pod, err := ldm.serviceIdResolver.ResolveIntentTargetToPod(ctx, target, clientIntents.Namespace)
		if err != nil {
			return nil, err
		}

		err = ldm.createIntentPrimaryResources(ctx, *clientIntents, clientServiceAccount)
		if err != nil {
			return nil, err
		}

		var ports []int32
		for _, container := range pod.Spec.Containers {
			if container.Name != LinkerdContainer {
				for _, port := range container.Ports {
					ports = append(ports, port.ContainerPort)
				}
			}
		}

		for _, port := range ports {
			logrus.Info("processing port: ", port)
			s, shouldCreateServer, err := ldm.shouldCreateServer(ctx, *clientIntents, target, port)
			if err != nil {
				return nil, err
			}
			logrus.Info("should create server result for port: ", port, shouldCreateServer)

			if shouldCreateServer {
				podSelector := ldm.BuildPodLabelSelectorFromIntent(target, clientIntents.Namespace)
				s, err = ldm.generateLinkerdServer(ctx, *clientIntents, target, podSelector, port)
				err = ldm.Client.Create(ctx, s)
				if err != nil {
					ldm.recorder.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd server: %s", err.Error())
					return nil, err
				}
			}
			currentResources.Servers.Add(s.UID)

			httpResources := target.GetHTTPResources()
			if len(httpResources) > 0 {
				probePath, err := ldm.getLivenessProbePath(pod) // should be get livenessprobepath for container with port
				if err != nil {
					return nil, err
				}

				if probePath != "" {
					httpRouteName := fmt.Sprintf(HTTPRouteNameTemplate, target.GetTargetServerName(), port, generateRandomString(8))
					probePathRoute, shouldCreateRoute, err := ldm.shouldCreateHTTPRoute(ctx, *clientIntents,
						probePath, s.Name)
					if err != nil {
						return nil, err
					}

					if shouldCreateRoute {
						probePathRoute, err = ldm.generateHTTPRoute(ctx, *clientIntents, s.Name, probePath,
							httpRouteName,
							clientIntents.Namespace)
						if err != nil {
							return nil, err
						}
						err = ldm.Client.Create(ctx, probePathRoute)
						if err != nil {
							return nil, err
						}
					}
					currentResources.Routes.Add(probePathRoute.UID)

					policy, shouldCreatePolicy, err := ldm.shouldCreateAuthPolicy(ctx,
						*clientIntents, probePathRoute.Name,
						LinkerdHTTPRouteKindName,
						NetworkAuthenticationNameTemplate,
						LinkerdNetAuthKindName)
					if err != nil {
						return nil, err
					}

					if shouldCreatePolicy {
						policy, err = ldm.generateAuthorizationPolicy(ctx, *clientIntents, target,
							port,
							httpRouteName,
							LinkerdHTTPRouteKindName,
							LinkerdNetAuthKindName)
						if err != nil {
							return nil, errors.Wrap(err)
						}
						err = ldm.Client.Create(ctx, policy)
						if err != nil {
							return nil, err
						}
					}

					currentResources.AuthorizationPolicies.Add(policy.UID)
				}

				for _, httpResource := range httpResources {
					httpRouteName := fmt.Sprintf(HTTPRouteNameTemplate, target.GetTargetServerName(), port, generateRandomString(8))
					route, shouldCreateRoute, err := ldm.shouldCreateHTTPRoute(ctx, *clientIntents,
						httpResource.Path, s.Name)
					if err != nil {
						return nil, err
					}

					if shouldCreateRoute {
						route, err = ldm.generateHTTPRoute(ctx, *clientIntents, s.Name, httpResource.Path,
							httpRouteName,
							clientIntents.Namespace)
						if err != nil {
							return nil, err
						}
						err = ldm.Client.Create(ctx, route)
						if err != nil {
							return nil, err
						}
					}
					currentResources.Routes.Add(route.UID)
					policy, shouldCreatePolicy, err := ldm.shouldCreateAuthPolicy(ctx,
						*clientIntents, route.Name,
						LinkerdHTTPRouteKindName,
						"meshtls-for-client-"+clientIntents.GetWorkloadName(),
						LinkerdMeshTLSAuthenticationKindName)
					if err != nil {
						return nil, err
					}

					if shouldCreatePolicy {
						policy, err = ldm.generateAuthorizationPolicy(ctx, *clientIntents, target,
							port,
							httpRouteName,
							LinkerdHTTPRouteKindName,
							LinkerdMeshTLSAuthenticationKindName)
						if err != nil {
							return nil, err
						}
						err = ldm.Client.Create(ctx, policy)
						if err != nil {
							ldm.recorder.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd policy: %s", err.Error())
							return nil, err
						}
					}
					currentResources.AuthorizationPolicies.Add(policy.UID)
				}
			} else {
				policy, shouldCreatePolicy, err := ldm.shouldCreateAuthPolicy(ctx, *clientIntents,
					s.Name,
					LinkerdServerKindName,
					"meshtls-for-client-"+clientIntents.GetWorkloadName(),
					LinkerdMeshTLSAuthenticationKindName)
				if err != nil {
					return nil, err
				}

				if shouldCreatePolicy {
					policy, err = ldm.generateAuthorizationPolicy(ctx, *clientIntents, target,
						port,
						s.Name,
						LinkerdServerKindName,
						LinkerdMeshTLSAuthenticationKindName)
					if err != nil {
						return nil, err
					}
					err = ldm.Client.Create(ctx, policy)
					if err != nil {
						ldm.recorder.RecordWarningEventf(clientIntents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd policy: %s", err.Error())
						return nil, err
					}
				}
				currentResources.AuthorizationPolicies.Add(policy.UID)
			}
		}
	}
	return currentResources, nil
}

func (ldm *LinkerdManager) BuildPodLabelSelectorFromIntent(target otterizev2alpha1.Target, intentsObjNamespace string) metav1.LabelSelector {
	otterizeServerLabel := map[string]string{}
	targetServerIdentity := target.ToServiceIdentity(intentsObjNamespace)

	formattedTargetServer := targetServerIdentity.GetFormattedOtterizeIdentityWithoutKind()
	otterizeServerLabel[otterizev2alpha1.OtterizeServiceLabelKey] = formattedTargetServer

	return metav1.LabelSelector{MatchLabels: otterizeServerLabel}
}

func (ldm *LinkerdManager) getServerName(intent otterizev2alpha1.Target, port int32) string {
	name := intent.GetTargetServerName()
	return fmt.Sprintf(OtterizeLinkerdServerNameTemplate, name, port)
}

func (ldm *LinkerdManager) createIntentPrimaryResources(ctx context.Context,
	intents otterizev2alpha1.ClientIntents,
	clientServiceAccount string) error {
	shouldCreateNetAuth, err := ldm.shouldCreateNetAuth(ctx, intents)
	if err != nil {
		return err
	}

	if shouldCreateNetAuth {
		netAuth, err := ldm.generateNetworkAuthentication(ctx, intents)
		if err != nil {
			return err
		}
		err = ldm.Client.Create(ctx, netAuth)
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
		mtls, err := ldm.generateMeshTLS(ctx, intents, []string{fullServiceAccountName})
		if err != nil {
			return err
		}
		err = ldm.Client.Create(ctx, mtls)
		if err != nil {
			ldm.recorder.RecordWarningEventf(&intents, ReasonCreatingLinkerdPolicyFailed, "Failed to create Linkerd meshTLS: %s", err.Error())
			return err
		}
	}
	return nil
}

func (ldm *LinkerdManager) getLivenessProbePath(pod corev1.Pod) (string, error) {
	// TODO: check of the other probe types will break
	for _, c := range pod.Spec.Containers {
		if c.Name != LinkerdContainer {
			if c.LivenessProbe != nil && c.LivenessProbe.HTTPGet != nil {
				return c.LivenessProbe.HTTPGet.Path, nil
			}
		}
	}
	return "", nil
}

func getPathMatchPointer(ap authpolicy.PathMatchType) *authpolicy.PathMatchType {
	return &ap
}

func (ldm *LinkerdManager) shouldCreateServer(ctx context.Context, intents otterizev2alpha1.ClientIntents, target otterizev2alpha1.Target, port int32) (*linkerdserver.Server, bool, error) {
	podSelector := ldm.BuildPodLabelSelectorFromIntent(target, intents.Namespace)
	servers := &linkerdserver.ServerList{}
	logrus.Infof("should create server ? %s, %d", podSelector.String(), port)
	// list all servers in the namespace
	err := ldm.Client.List(ctx, servers, &client.ListOptions{Namespace: intents.Namespace})
	if err != nil {
		return nil, false, err
	}

	// no servers exist
	if len(servers.Items) == 0 {
		logrus.Info("no servers in the list")
		return nil, true, nil
	}

	// get servers in the namespace and if any of them has a label selector similar to the intents label return that server
	for _, server := range servers.Items {
		if server.Name == fmt.Sprintf(OtterizeLinkerdServerNameTemplate, target.GetTargetServerName(), port) {
			// check if it has the annotation of this service if it doesnt add it
			return &server, false, nil
		}
	}
	return nil, true, nil
}

func (ldm *LinkerdManager) shouldCreateHTTPRoute(ctx context.Context,
	intents otterizev2alpha1.ClientIntents,
	path,
	parentName string,
) (*authpolicy.HTTPRoute, bool, error) {
	routes := &authpolicy.HTTPRouteList{}
	err := ldm.Client.List(ctx, routes, &client.ListOptions{Namespace: intents.Namespace})
	if err != nil {
		return nil, false, err
	}
	// Dont create the route if it has the same parent server as the serve in question and defines the same rule
	for _, route := range routes.Items {
		for _, parent := range route.Spec.ParentRefs {
			if parent.Name == v1beta1.ObjectName(parentName) {
				for _, rule := range route.Spec.Rules {
					for _, match := range rule.Matches {
						if *match.Path.Value == path {
							return &route, false, nil
						}
					}
				}
			}
		}
	}
	return nil, true, nil
}

func (ldm *LinkerdManager) shouldCreateAuthPolicy(ctx context.Context,
	intents otterizev2alpha1.ClientIntents,
	targetName,
	targetRefKind,
	authRefName,
	authRefKind string) (*authpolicy.AuthorizationPolicy, bool, error) {
	authPolicies := &authpolicy.AuthorizationPolicyList{}

	err := ldm.Client.List(ctx, authPolicies, &client.ListOptions{Namespace: intents.Namespace})
	if err != nil {
		return nil, false, err
	}
	for _, policy := range authPolicies.Items {
		if policy.Spec.TargetRef.Name == v1beta1.ObjectName(targetName) && policy.Spec.TargetRef.Kind == v1beta1.Kind(targetRefKind) {
			for _, authRef := range policy.Spec.RequiredAuthenticationRefs {
				if authRef.Kind == v1beta1.Kind(authRefKind) && authRef.Name == v1beta1.ObjectName(authRefName) {
					logrus.Infof("not creating policy for policy with details, %s, %s", policy.Spec.TargetRef.Name, authRef.Name)
					return &policy, false, nil
				}
			}
		}
	}
	return nil, true, nil
}

func (ldm *LinkerdManager) shouldCreateMeshTLS(ctx context.Context, clientIntents otterizev2alpha1.ClientIntents) (bool, error) {
	meshes := &authpolicy.MeshTLSAuthenticationList{}
	err := ldm.Client.List(ctx, meshes, &client.ListOptions{Namespace: clientIntents.Namespace})
	if err != nil {
		return false, err
	}
	for _, mesh := range meshes.Items {
		if mesh.Name == fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, clientIntents.GetWorkloadName()) {
			return false, nil
		}
	}
	return true, nil
}

func (ldm *LinkerdManager) shouldCreateNetAuth(ctx context.Context, intents otterizev2alpha1.ClientIntents) (bool, error) {
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

func (ldm *LinkerdManager) DeleteResourceIfNotReferencedByOtherPolicy(ctx context.Context, object client.Object, policies map[string]authpolicy.AuthorizationPolicy, isTargetRef bool) error {
	// go through all policies, if another policy has this route as a target ref, update the server annotation
	// of that route to be equal to that of the policy and don't delete the route
	for _, policy := range policies {
		logrus.Info("Name of policy: ", policy.Spec.TargetRef.Name, " ", "Name of object", object.GetName())
		if isTargetRef {
			if string(policy.Spec.TargetRef.Name) == object.GetName() {
				object.GetLabels()[otterizev2alpha1.OtterizeLinkerdServerAnnotationKey] = policy.Labels[otterizev2alpha1.OtterizeLinkerdServerAnnotationKey]
				logrus.Info("Updating object with name ", object.GetName(), "to be owned by ", policy.Labels[otterizev2alpha1.OtterizeLinkerdServerAnnotationKey])
				err := ldm.Client.Update(ctx, object, &client.UpdateOptions{})
				if err != nil {
					return err
				}
				continue
			}
			logrus.Info("deleting object ", object.GetName(), "owned by", object.GetLabels()[otterizev2alpha1.OtterizeLinkerdServerAnnotationKey])
			err := ldm.Client.Delete(ctx, object)
			if err != nil {
				return err
			}
			return nil
		}
		for _, authRef := range policy.Spec.RequiredAuthenticationRefs {
			if string(authRef.Name) == object.GetName() {
				object.GetLabels()[otterizev2alpha1.OtterizeLinkerdServerAnnotationKey] = policy.Labels[otterizev2alpha1.OtterizeLinkerdServerAnnotationKey]
				logrus.Info("Updating object with name ", object.GetName(), "to be owned by ", policy.Labels[otterizev2alpha1.OtterizeLinkerdServerAnnotationKey])
				err := ldm.Client.Update(ctx, object, &client.UpdateOptions{})
				if err != nil {
					return err
				}
				continue
			}
			logrus.Info("deleting object ", object.GetName(), "owned by ", object.GetLabels()[otterizev2alpha1.OtterizeLinkerdServerAnnotationKey])
			err := ldm.Client.Delete(ctx, object)
			if err != nil {
				return err
			}
			return nil
		}
	}
	return nil
}

func (ldm *LinkerdManager) generateLinkerdServer(ctx context.Context, intents otterizev2alpha1.ClientIntents, target otterizev2alpha1.Target, podSelector metav1.LabelSelector, port int32) (*linkerdserver.Server, error) {
	name := ldm.getServerName(target, port)
	serverNamespace := target.GetTargetServerNamespace(intents.Namespace)

	identity, err := ldm.getFormattedOtterizeIdentityFromIntents(ctx, intents)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	s := linkerdserver.Server{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1beta1",
			Kind:       "Server",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: serverNamespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: identity,
			},
		},
		Spec: linkerdserver.ServerSpec{
			PodSelector: &podSelector,
			Port:        intstr.FromInt32(port),
		},
	}
	return &s, nil
}

func (ldm *LinkerdManager) generateAuthorizationPolicy(
	ctx context.Context,
	clientIntents otterizev2alpha1.ClientIntents,
	target otterizev2alpha1.Target,
	port int32,
	serverTargetName,
	targetRefType,
	requiredAuthRefType string,
) (*authpolicy.AuthorizationPolicy, error) {
	var (
		targetRefName v1beta1.ObjectName
		policyName    = fmt.Sprintf(OtterizeLinkerdAuthPolicyNameTemplate, target.GetTargetServerName(), port, clientIntents.GetWorkloadName(), generateRandomString(8))
	)
	switch requiredAuthRefType {
	case LinkerdNetAuthKindName:
		targetRefName = NetworkAuthenticationNameTemplate
	case LinkerdMeshTLSAuthenticationKindName:
		targetRefName = v1beta1.ObjectName(fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, clientIntents.GetWorkloadName()))
	}

	identity, err := ldm.getFormattedOtterizeIdentityFromIntents(ctx, clientIntents)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return &authpolicy.AuthorizationPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1alpha1",
			Kind:       "AuthorizationPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: clientIntents.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: identity,
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
	}, nil
}

func StringPtr(s string) *string {
	return &s
}

func (ldm *LinkerdManager) generateHTTPRoute(ctx context.Context, intents otterizev2alpha1.ClientIntents,
	serverName, path, name, namespace string) (*authpolicy.HTTPRoute, error) {
	identity, err := ldm.getFormattedOtterizeIdentityFromIntents(ctx, intents)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return &authpolicy.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1beta3",
			Kind:       "HTTPRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: identity,
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
	}, nil
}

func (ldm *LinkerdManager) generateMeshTLS(
	ctx context.Context,
	intents otterizev2alpha1.ClientIntents,
	targets []string,
) (*authpolicy.MeshTLSAuthentication, error) {
	ot, err := ldm.getFormattedOtterizeIdentityFromIntents(ctx, intents)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	mtls := authpolicy.MeshTLSAuthentication{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1alpha1",
			Kind:       "MeshTLSAuthentication",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, intents.GetWorkloadName()),
			Namespace: intents.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: ot,
			},
		},
		Spec: authpolicy.MeshTLSAuthenticationSpec{
			Identities: targets,
		},
	}
	return &mtls, nil
}

func (ldm *LinkerdManager) generateNetworkAuthentication(ctx context.Context, intents otterizev2alpha1.ClientIntents) (*authpolicy.NetworkAuthentication, error) {
	identity, err := ldm.getFormattedOtterizeIdentityFromIntents(ctx, intents)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	return &authpolicy.NetworkAuthentication{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policy.linkerd.io/v1alpha1",
			Kind:       "NetworkAuthentication",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      NetworkAuthenticationNameTemplate,
			Namespace: intents.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: identity,
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
	}, nil
}

func (ldm *LinkerdManager) getFormattedOtterizeIdentityFromIntents(ctx context.Context, clientIntents otterizev2alpha1.ClientIntents) (string, error) {
	pod, err := ldm.serviceIdResolver.ResolveClientIntentToPod(ctx, clientIntents)
	if err != nil {
		return "", errors.Wrap(err)
	}
	svcIdentity, err := ldm.serviceIdResolver.ResolvePodToServiceIdentity(ctx, &pod)
	if err != nil {
		return "", errors.Wrap(err)
	}
	return svcIdentity.GetFormattedOtterizeIdentityWithoutKind(), nil
}
