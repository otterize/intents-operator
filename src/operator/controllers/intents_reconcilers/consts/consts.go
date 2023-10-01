package consts

// Consts have to go here to prevent import cycle between istiopolicy and intents_reconcilers.
const (
	ReasonEnforcementDefaultOff               = "EnforcementGloballyDisabled"
	ReasonNetworkPolicyCreationDisabled       = "NetworkPolicyCreationDisabled"
	ReasonGettingNetworkPolicyFailed          = "GettingNetworkPolicyFailed"
	ReasonRemovingNetworkPolicyFailed         = "RemovingNetworkPolicyFailed"
	ReasonNamespaceNotAllowed                 = "NamespaceNotAllowed"
	ReasonCreatingNetworkPoliciesFailed       = "CreatingNetworkPoliciesFailed"
	ReasonCreatedNetworkPolicies              = "CreatedNetworkPolicies"
	ReasonEgressNetworkPolicyCreationDisabled = "EgressNetworkPolicyCreationDisabled"
	ReasonGettingEgressNetworkPolicyFailed    = "GettingEgressNetworkPolicyFailed"
	ReasonRemovingEgressNetworkPolicyFailed   = "RemovingEgressNetworkPolicyFailed"
	ReasonCreatingEgressNetworkPoliciesFailed = "CreatingEgressNetworkPoliciesFailed"
	ReasonCreatedEgressNetworkPolicies        = "CreatedEgressNetworkPolicies"
	IstioPolicyFinalizerName                  = "intents.otterize.com/istio-policy-finalizer"
	ReasonIstioPolicyCreationDisabled         = "IstioPolicyCreationDisabled"
	ReasonRemovingIstioPolicyFailed           = "RemovingIstioPolicyFailed"
	ReasonOtterizeServiceNotFound             = "OtterizeServiceNotFound"
)
