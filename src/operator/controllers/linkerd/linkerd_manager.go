package linkerdmanager

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	authpolicy "github.com/linkerd/linkerd2/controller/gen/apis/policy/v1alpha1"
	linkerdserver "github.com/linkerd/linkerd2/controller/gen/apis/server/v1beta1"
	"github.com/otterize/intents-operator/src/operator/controllers/intents_reconcilers/protected_services"
	"github.com/otterize/intents-operator/src/operator/effectivepolicy"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"

	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
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
	OtterizeLinkerdServerNameTemplate     = "server-for-%s-port-%d"
	OtterizeLinkerdMeshTLSNameTemplate    = "meshtls-for-client-%s"
	OtterizeLinkerdAuthPolicyNameTemplate = "authpolicy-to-%s-port-%d-from-client-%s-%s"
	ReasonCreatedLinkerdServer            = "CreatedLinkerdServer"
	ReasonCreatedLinkerdAuthPolicy        = "CreatedLinkerdAuthorizationPolicy"
	ReasonNamespaceNotAllowed             = "NamespaceNotAllowed"
	ReasonNotPartOfLinkerdMesh            = "NotPartOfLinkerdMesh"
	FullServiceAccountName                = "%s.%s.serviceaccount.identity.linkerd.cluster.local"
	NetworkAuthenticationNameTemplate     = "network-auth-for-probe-routes"
	HTTPRouteNameTemplate                 = "http-route-for-%s-port-%d-%s"
	LinkerdMeshTLSAuthenticationKindName  = "MeshTLSAuthentication"
	LinkerdServerKindName                 = "Server"
	LinkerdHTTPRouteKindName              = "HTTPRoute"
	LinkerdNetAuthKindName                = "NetworkAuthentication"
	LinkerdContainer                      = "linkerd-proxy"
)

//+kubebuilder:rbac:groups="policy.linkerd.io",resources=authorizationpolicies,verbs=get;update;patch;list;watch;delete;create;deletecollection
//+kubebuilder:rbac:groups="policy.linkerd.io",resources=meshtlsauthentications,verbs=get;update;patch;list;watch;delete;create;deletecollection
//+kubebuilder:rbac:groups="policy.linkerd.io",resources=networkauthentications,verbs=get;update;patch;list;watch;delete;create;deletecollection
//+kubebuilder:rbac:groups="policy.linkerd.io",resources=servers,verbs=get;update;patch;list;watch;delete;create;deletecollection
//+kubebuilder:rbac:groups="policy.linkerd.io",resources=httproutes,verbs=get;update;patch;list;watch;delete;create;deletecollection
//+kubebuilder:rbac:groups="policy.linkerd.io",resources=httproutes,verbs=get;update;patch;list;watch;delete;create;deletecollection

type LinkerdPolicyManager interface {
	DeleteOutdatedResources(ctx context.Context, eps []effectivepolicy.ServiceEffectivePolicy, validResources LinkerdResourceMapping) error
	CreateResources(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy, clientServiceAccount string) (*LinkerdResourceMapping, error)
}

type LinkerdResourceMapping struct {
	Servers               *goset.Set[types.UID]
	AuthorizationPolicies *goset.Set[types.UID]
	Routes                *goset.Set[types.UID]
}

type LinkerdManager struct {
	client.Client
	serviceIdResolver       serviceidresolver.ServiceResolver
	recorder                *injectablerecorder.InjectableRecorder
	restrictedToNamespaces  []string
	enforcementDefaultState bool
}

func NewLinkerdManager(c client.Client,
	namespaces []string,
	r *injectablerecorder.InjectableRecorder,
	enforcementDefaultState bool) *LinkerdManager {

	return &LinkerdManager{
		Client:                  c,
		serviceIdResolver:       serviceidresolver.NewResolver(c),
		restrictedToNamespaces:  namespaces,
		recorder:                r,
		enforcementDefaultState: enforcementDefaultState,
	}
}

func (ldm *LinkerdManager) DeleteOutdatedResources(ctx context.Context, eps []effectivepolicy.ServiceEffectivePolicy, validResources LinkerdResourceMapping) error {
	for _, ep := range eps {
		clientFormattedIdentity := ep.Service.GetFormattedOtterizeIdentityWithoutKind()

		var (
			existingPolicies   authpolicy.AuthorizationPolicyList
			existingServers    linkerdserver.ServerList
			existingHttpRoutes authpolicy.HTTPRouteList
			existingNetAuths   authpolicy.NetworkAuthenticationList
			existingMTLS       authpolicy.MeshTLSAuthenticationList
		)

		err := ldm.Client.List(ctx, &existingPolicies, client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
		if err != nil {
			return errors.Wrap(err)
		}

		err = ldm.Client.List(ctx,
			&existingServers,
			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
		if err != nil {
			return errors.Wrap(err)
		}

		err = ldm.Client.List(ctx,
			&existingHttpRoutes,
			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
		if err != nil {
			return errors.Wrap(err)
		}

		err = ldm.Client.List(ctx,
			&existingNetAuths,
			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
		if err != nil {
			return errors.Wrap(err)
		}

		err = ldm.Client.List(ctx,
			&existingMTLS,
			client.MatchingLabels{otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: clientFormattedIdentity})
		if err != nil {
			return errors.Wrap(err)
		}

		for _, existingPolicy := range existingPolicies.Items {
			if !validResources.AuthorizationPolicies.Contains(existingPolicy.UID) {
				err := ldm.Client.Delete(ctx, &existingPolicy)
				if err != nil {
					return errors.Wrap(err)
				}
			}
		}

		for _, existingServer := range existingServers.Items {
			if !validResources.Servers.Contains(existingServer.UID) {
				err := ldm.Client.Delete(ctx, &existingServer)
				if err != nil {
					return errors.Wrap(err)
				}
			}
		}

		for _, existingRoute := range existingHttpRoutes.Items {
			if !validResources.Routes.Contains(existingRoute.UID) {
				err := ldm.Client.Delete(ctx, &existingRoute)
				if err != nil {
					return errors.Wrap(err)
				}
			}
		}
	}

	return nil
}

func (ldm *LinkerdManager) CreateResources(ctx context.Context, ep effectivepolicy.ServiceEffectivePolicy, clientServiceAccount string) (*LinkerdResourceMapping, error) {
	currentResources := &LinkerdResourceMapping{
		Servers:               goset.NewSet[types.UID](),
		AuthorizationPolicies: goset.NewSet[types.UID](),
		Routes:                goset.NewSet[types.UID](),
	}

	for _, target := range ep.Calls {
		if !target.IsTargetInCluster() { // this will skip non http ones, db for example, skip port doesn't exist as well
			continue
		}
		clientNamespace := ep.Service.Namespace
		pod, err := ldm.serviceIdResolver.ResolveIntentTargetToPod(ctx, target.Target, clientNamespace)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		shouldCreateLinkerdResources, err := protected_services.IsServerEnforcementEnabledDueToProtectionOrDefaultState(
			ctx, ldm.Client, ep.Service, ldm.enforcementDefaultState, goset.FromSlice(ldm.restrictedToNamespaces))
		if err != nil {
			return nil, errors.Wrap(err)
		}

		if !shouldCreateLinkerdResources {
			logrus.Debugf("Enforcement is disabled globally and server is not explicitly protected, skipping linkerd policy creation for server %s in namespace %s", target.GetTargetServerName(), target.GetTargetServerNamespace(clientNamespace))
			continue
		}

		targetNamespace := target.GetTargetServerNamespace(clientNamespace)
		if len(ldm.restrictedToNamespaces) != 0 && !lo.Contains(ldm.restrictedToNamespaces, targetNamespace) {
			ep.ClientIntentsEventRecorder.RecordWarningEventf(
				ReasonNamespaceNotAllowed,
				"Namespace %s was specified in intent, but is not allowed by configuration, Linkerd policy ignored",
				targetNamespace)

			logrus.Warningf(
				"Namespace %s was specified in intent, but is not allowed by configuration, Linkerd policy ignored",
				targetNamespace,
			)
			continue
		}

		pod, err = ldm.serviceIdResolver.ResolveIntentTargetToPod(ctx, target.Target, clientNamespace)
		if err != nil {
			return nil, errors.Wrap(err)
		}

		err = ldm.createIntentPrimaryResources(ctx, ep.Service, clientServiceAccount)
		if err != nil {
			return nil, errors.Wrap(err)
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
			server, shouldCreateServer, err := ldm.shouldCreateServer(ctx, ep.Service, target.Target, port)
			if err != nil {
				return nil, errors.Wrap(err)
			}
			logrus.Infof("should create server result for port %d - %t", port, shouldCreateServer)

			if shouldCreateServer {
				podSelector := ldm.BuildPodLabelSelectorFromTarget(target.Target, clientNamespace)
				server = ldm.generateLinkerdServer(ep.Service, target.Target, podSelector, port)
				err = ldm.Client.Create(ctx, server)
				if err != nil {
					logrus.Errorf("Failed to create Linkerd server: %s", err.Error())
					return nil, errors.Wrap(err)
				}
				ep.ClientIntentsEventRecorder.RecordNormalEventf(ReasonCreatedLinkerdServer, "Successfully created Linkerd server: %s", server.Name)
			}
			currentResources.Servers.Add(server.UID)

			httpResources := target.GetHTTPResources()
			if len(httpResources) > 0 {
				probePath, err := ldm.getLivenessProbePath(pod) // should be get livenessprobepath for container with port
				if err != nil {
					return nil, errors.Wrap(err)
				}

				if probePath != "" {
					if err := ldm.handleLivenessProbResources(ctx, probePath, clientNamespace, server.Name, ep.Service, target, port, currentResources); err != nil {
						return nil, errors.Wrap(err)
					}
				}

				for _, httpResource := range httpResources {
					if err := ldm.handleHTTPResource(ctx, clientNamespace, server.Name, target, ep.Service, httpResource, port, currentResources); err != nil {
						return nil, errors.Wrap(err)
					}
				}
			} else {
				policy, shouldCreatePolicy, err := ldm.shouldCreateAuthPolicy(ctx, ep.Service,
					server.Name,
					LinkerdServerKindName,
					"meshtls-for-client-"+ep.Service.GetName(),
					LinkerdMeshTLSAuthenticationKindName)
				if err != nil {
					return nil, errors.Wrap(err)
				}

				if shouldCreatePolicy {
					policy = ldm.generateAuthorizationPolicy(ep.Service, target.Target,
						port,
						server.Name,
						LinkerdServerKindName,
						LinkerdMeshTLSAuthenticationKindName)
					err = ldm.Client.Create(ctx, policy)
					if err != nil {
						logrus.Errorf("Failed to create Linkerd policy: %s", err.Error())
						return nil, errors.Wrap(err)
					}
					ep.ClientIntentsEventRecorder.RecordNormalEvent(ReasonCreatedLinkerdAuthPolicy, "Successfully created Linkerd AuthorizationPolicy")
				}
				currentResources.AuthorizationPolicies.Add(policy.UID)
			}
		}
	}

	return currentResources, nil
}

func (ldm *LinkerdManager) BuildPodLabelSelectorFromTarget(target otterizev2alpha1.Target, intentsObjNamespace string) metav1.LabelSelector {
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

func (ldm *LinkerdManager) createIntentPrimaryResources(ctx context.Context, svcIdentity serviceidentity.ServiceIdentity, clientServiceAccount string) error {
	shouldCreateNetAuth, err := ldm.shouldCreateNetAuth(ctx, svcIdentity)
	if err != nil {
		return err
	}

	if shouldCreateNetAuth {
		netAuth, err := ldm.generateNetworkAuthentication(svcIdentity)
		if err != nil {
			return err
		}
		err = ldm.Client.Create(ctx, netAuth)
		if err != nil {
			return err
		}
	}

	shouldCreateMeshTLS, err := ldm.shouldCreateMeshTLS(ctx, svcIdentity)
	if err != nil {
		return err
	}
	fullServiceAccountName := fmt.Sprintf(FullServiceAccountName, clientServiceAccount, svcIdentity.Namespace)

	if shouldCreateMeshTLS {
		mtls, err := ldm.generateMeshTLS(svcIdentity, []string{fullServiceAccountName})
		if err != nil {
			return errors.Wrap(err)
		}
		err = ldm.Client.Create(ctx, mtls)
		if err != nil {
			return errors.Wrap(err)
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

func (ldm *LinkerdManager) shouldCreateServer(ctx context.Context, svcIdentity serviceidentity.ServiceIdentity, target otterizev2alpha1.Target, port int32) (*linkerdserver.Server, bool, error) {
	logrus.Infof("should create server ? %s, %d", target.ToServiceIdentity(svcIdentity.Namespace).GetFormattedOtterizeIdentityWithoutKind(), port)
	servers := &linkerdserver.ServerList{}
	// list all servers in the namespace
	err := ldm.Client.List(ctx, servers, &client.ListOptions{Namespace: svcIdentity.Namespace})
	if err != nil {
		return nil, false, errors.Wrap(err)
	}

	// no servers exist
	if len(servers.Items) == 0 {
		logrus.Info("no servers in the list")
		return nil, true, nil
	}

	// get servers in the namespace and if any of them has a label selector similar to the intents label return that server
	for _, server := range servers.Items {
		if server.Name == fmt.Sprintf(OtterizeLinkerdServerNameTemplate, target.GetTargetServerName(), port) {
			// check if it has the annotation of this service if it doesn't, add it
			return &server, false, nil
		}
	}
	return nil, true, nil
}

func (ldm *LinkerdManager) shouldCreateHTTPRoute(ctx context.Context, svcIdentity serviceidentity.ServiceIdentity, path, parentName string) (*authpolicy.HTTPRoute, bool, error) {
	routes := &authpolicy.HTTPRouteList{}
	err := ldm.Client.List(ctx, routes, &client.ListOptions{Namespace: svcIdentity.Namespace})
	if err != nil {
		return nil, false, err
	}
	// Don't create the route if it has the same parent server as the serve in question and defines the same rule
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

func (ldm *LinkerdManager) shouldCreateAuthPolicy(ctx context.Context, svcIdentity serviceidentity.ServiceIdentity, targetName, targetRefKind, authRefName, authRefKind string) (*authpolicy.AuthorizationPolicy, bool, error) {
	authPolicies := &authpolicy.AuthorizationPolicyList{}

	err := ldm.Client.List(ctx, authPolicies, &client.ListOptions{Namespace: svcIdentity.Namespace})
	if err != nil {
		return nil, false, err
	}
	for _, policy := range authPolicies.Items {
		if policy.Spec.TargetRef.Name == v1beta1.ObjectName(targetName) && policy.Spec.TargetRef.Kind == v1beta1.Kind(targetRefKind) {
			for _, authRef := range policy.Spec.RequiredAuthenticationRefs {
				if authRef.Kind == v1beta1.Kind(authRefKind) && authRef.Name == v1beta1.ObjectName(authRefName) {
					logrus.Debugf("not creating policy for policy with details, %s, %s", policy.Spec.TargetRef.Name, authRef.Name)
					return &policy, false, nil
				}
			}
		}
	}
	return nil, true, nil
}

func (ldm *LinkerdManager) shouldCreateMeshTLS(ctx context.Context, svcIdentity serviceidentity.ServiceIdentity) (bool, error) {
	meshes := &authpolicy.MeshTLSAuthenticationList{}
	err := ldm.Client.List(ctx, meshes, &client.ListOptions{Namespace: svcIdentity.Namespace})
	if err != nil {
		return false, err
	}
	for _, mesh := range meshes.Items {
		if mesh.Name == fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, svcIdentity.GetName()) {
			return false, nil
		}
	}
	return true, nil
}

func (ldm *LinkerdManager) shouldCreateNetAuth(ctx context.Context, svcIdentity serviceidentity.ServiceIdentity) (bool, error) {
	netauths := &authpolicy.NetworkAuthenticationList{}
	err := ldm.Client.List(ctx, netauths, &client.ListOptions{Namespace: svcIdentity.Namespace})
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

func (ldm *LinkerdManager) generateLinkerdServer(svcIdentity serviceidentity.ServiceIdentity, target otterizev2alpha1.Target, podSelector metav1.LabelSelector, port int32) *linkerdserver.Server {
	name := ldm.getServerName(target, port)
	serverNamespace := target.GetTargetServerNamespace(svcIdentity.Namespace)

	s := linkerdserver.Server{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: serverNamespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
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
	svcIdentity serviceidentity.ServiceIdentity,
	target otterizev2alpha1.Target,
	port int32,
	serverTargetName,
	targetRefType,
	requiredAuthRefType string,
) *authpolicy.AuthorizationPolicy {
	var (
		targetRefName v1beta1.ObjectName
		policyName    = fmt.Sprintf(OtterizeLinkerdAuthPolicyNameTemplate, target.GetTargetServerName(), port, svcIdentity.GetName(), generateRandomString(8))
	)
	switch requiredAuthRefType {
	case LinkerdNetAuthKindName:
		targetRefName = NetworkAuthenticationNameTemplate
	case LinkerdMeshTLSAuthenticationKindName:
		targetRefName = v1beta1.ObjectName(fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, svcIdentity.GetName()))
	}

	otterizeIdentity := svcIdentity.GetFormattedOtterizeIdentityWithoutKind()
	return &authpolicy.AuthorizationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: svcIdentity.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: otterizeIdentity,
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
}

func StringPtr(s string) *string {
	return &s
}

func (ldm *LinkerdManager) generateHTTPRoute(svcIdentity serviceidentity.ServiceIdentity,
	serverName, path, name, namespace string) (*authpolicy.HTTPRoute, error) {

	return &authpolicy.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
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
							//Method: getHTTPMethodPointer(authpolicy.HTTPMethodGet), add support for that later
						},
					},
				},
			},
		},
	}, nil
}

func (ldm *LinkerdManager) generateMeshTLS(svcIdentity serviceidentity.ServiceIdentity, targets []string) (*authpolicy.MeshTLSAuthentication, error) {
	mtls := authpolicy.MeshTLSAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(OtterizeLinkerdMeshTLSNameTemplate, svcIdentity.GetName()),
			Namespace: svcIdentity.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
			},
		},
		Spec: authpolicy.MeshTLSAuthenticationSpec{
			Identities: targets,
		},
	}
	return &mtls, nil
}

func (ldm *LinkerdManager) generateNetworkAuthentication(svcIdentity serviceidentity.ServiceIdentity) (*authpolicy.NetworkAuthentication, error) {
	return &authpolicy.NetworkAuthentication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NetworkAuthenticationNameTemplate,
			Namespace: svcIdentity.Namespace,
			Labels: map[string]string{
				otterizev2alpha1.OtterizeLinkerdServerAnnotationKey: svcIdentity.GetFormattedOtterizeIdentityWithoutKind(),
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

func (ldm *LinkerdManager) handleLivenessProbResources(
	ctx context.Context,
	probePath, clientNamespace, serverName string,
	svcIdentity serviceidentity.ServiceIdentity,
	target effectivepolicy.Call,
	port int32,
	currentResources *LinkerdResourceMapping,
) error {
	httpRouteName := fmt.Sprintf(HTTPRouteNameTemplate, target.GetTargetServerName(), port, generateRandomString(8))
	probePathRoute, shouldCreateRoute, err := ldm.shouldCreateHTTPRoute(ctx, svcIdentity, probePath, serverName)
	if err != nil {
		return errors.Wrap(err)
	}

	if shouldCreateRoute {
		probePathRoute, err = ldm.generateHTTPRoute(svcIdentity, serverName, probePath,
			httpRouteName,
			clientNamespace)
		if err != nil {
			return errors.Wrap(err)
		}
		err = ldm.Client.Create(ctx, probePathRoute)
		if err != nil {
			return errors.Wrap(err)
		}
	}
	currentResources.Routes.Add(probePathRoute.UID)

	policy, shouldCreatePolicy, err := ldm.shouldCreateAuthPolicy(ctx,
		svcIdentity, probePathRoute.Name,
		LinkerdHTTPRouteKindName,
		NetworkAuthenticationNameTemplate,
		LinkerdNetAuthKindName)
	if err != nil {
		return errors.Wrap(err)
	}

	if shouldCreatePolicy {
		policy = ldm.generateAuthorizationPolicy(
			svcIdentity,
			target.Target,
			port,
			httpRouteName,
			LinkerdHTTPRouteKindName,
			LinkerdNetAuthKindName)

		err = ldm.Client.Create(ctx, policy)
		if err != nil {
			return errors.Wrap(err)
		}
	}

	currentResources.AuthorizationPolicies.Add(policy.UID)
	return nil
}

func (ldm *LinkerdManager) handleHTTPResource(
	ctx context.Context,
	clientNamespace,
	serverName string,
	target effectivepolicy.Call,
	svcIdentity serviceidentity.ServiceIdentity,
	httpResource otterizev2alpha1.HTTPTarget,
	port int32,
	currentResources *LinkerdResourceMapping,
) error {

	httpRouteName := ldm.getHTTPRouteName(target, port, clientNamespace, httpResource.Path)
	route, shouldCreateRoute, err := ldm.shouldCreateHTTPRoute(ctx, svcIdentity,
		httpResource.Path, serverName)
	if err != nil {
		return errors.Wrap(err)
	}

	if shouldCreateRoute {
		route, err = ldm.generateHTTPRoute(svcIdentity, serverName, httpResource.Path,
			httpRouteName,
			clientNamespace)
		if err != nil {
			return errors.Wrap(err)
		}
		err = ldm.Client.Create(ctx, route)
		if err != nil {
			return errors.Wrap(err)
		}
	}
	currentResources.Routes.Add(route.UID)
	policy, shouldCreatePolicy, err := ldm.shouldCreateAuthPolicy(ctx,
		svcIdentity, route.Name,
		LinkerdHTTPRouteKindName,
		"meshtls-for-client-"+svcIdentity.GetName(),
		LinkerdMeshTLSAuthenticationKindName)
	if err != nil {
		return errors.Wrap(err)
	}

	if shouldCreatePolicy {
		policy = ldm.generateAuthorizationPolicy(svcIdentity, target.Target,
			port,
			httpRouteName,
			LinkerdHTTPRouteKindName,
			LinkerdMeshTLSAuthenticationKindName)

		err = ldm.Client.Create(ctx, policy)
		if err != nil {
			logrus.Errorf("Failed to create Linkerd policy: %s", err.Error())
			return errors.Wrap(err)
		}
	}

	currentResources.AuthorizationPolicies.Add(policy.UID)
	return nil
}

func (ldm *LinkerdManager) getHTTPRouteName(target effectivepolicy.Call, port int32, clientNamespace, apiPath string) string {
	hash := md5.Sum([]byte(fmt.Sprintf("%s-%s-%d-%s", target.GetTargetServerName(), target.GetTargetServerNamespace(clientNamespace), port, apiPath)))
	return fmt.Sprintf(HTTPRouteNameTemplate, target.GetTargetServerName(), port, hex.EncodeToString(hash[:])[:6])
}
