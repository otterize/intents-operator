package consts

// Consts have to go here to prevent import cycle between istiopolicy and intents_reconcilers.
const (
	ReasonEnforcementDefaultOff         = "EnforcementGloballyDisabled"
	ReasonNetworkPolicyCreationDisabled = "NetworkPolicyCreationDisabled"
	ReasonGettingNetworkPolicyFailed    = "GettingNetworkPolicyFailed"
	ReasonRemovingNetworkPolicyFailed   = "RemovingNetworkPolicyFailed"
	ReasonNamespaceNotAllowed           = "NamespaceNotAllowed"
	ReasonCreatingNetworkPoliciesFailed = "CreatingNetworkPoliciesFailed"
	ReasonCreatedNetworkPolicies        = "CreatedNetworkPolicies"
	IstioPolicyFinalizerName            = "intents.otterize.com/istio-policy-finalizer"
	ReasonIstioPolicyCreationDisabled   = "IstioPolicyCreationDisabled"
	ReasonRemovingIstioPolicyFailed     = "RemovingIstioPolicyFailed"
	ReasonOtterizeServiceNotFound       = "OtterizeServiceNotFound"
)
