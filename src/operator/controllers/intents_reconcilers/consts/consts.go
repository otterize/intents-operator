package consts

// Consts have to go here to prevent import cycle between istiopolicy and intents_reconcilers.
const (
	ReasonEnforcementDefaultOff                         = "EnforcementGloballyDisabled"
	ReasonNetworkPolicyCreationDisabled                 = "NetworkPolicyCreationDisabled"
	ReasonGettingNetworkPolicyFailed                    = "GettingNetworkPolicyFailed"
	ReasonRemovingNetworkPolicyFailed                   = "RemovingNetworkPolicyFailed"
	ReasonNamespaceNotAllowed                           = "NamespaceNotAllowed"
	ReasonCreatingNetworkPoliciesFailed                 = "CreatingNetworkPoliciesFailed"
	ReasonCreatedNetworkPolicies                        = "CreatedNetworkPolicies"
	ReasonIstioPolicyCreationDisabled                   = "IstioPolicyCreationDisabled"
	ReasonRemovingIstioPolicyFailed                     = "RemovingIstioPolicyFailed"
	ReasonPodsNotFound                                  = "PodsNotFound"
	ReasonAWSIntentsFoundButNoServiceAccount            = "ReasonAWSIntentsFoundButNoServiceAccount"
	ReasonReconciledAWSPolicies                         = "ReasonReconciledAWSPolicies"
	ReasonReconcilingAWSPoliciesFailed                  = "ReasonReconcilingAWSPoliciesFailed"
	ReasonAWSIntentsServiceAccountUsedByMultipleClients = "ReasonAWSIntentsServiceAccountUsedByMultipleClients"
	ReasonKubernetesServiceNotFound                     = "KubernetesServiceNotFound"
	ReasonPortRestrictionUnsupportedForStrings          = "TypeStringPortNotSupported"
	ReasonEgressNetworkPolicyCreationDisabled           = "EgressNetworkPolicyCreationDisabled"
	ReasonGettingEgressNetworkPolicyFailed              = "GettingEgressNetworkPolicyFailed"
	ReasonRemovingEgressNetworkPolicyFailed             = "RemovingEgressNetworkPolicyFailed"
	ReasonCreatingEgressNetworkPoliciesFailed           = "CreatingEgressNetworkPoliciesFailed"
	ReasonCreatedEgressNetworkPolicies                  = "CreatedEgressNetworkPolicies"
	ReasonCreatedInternetEgressNetworkPolicies          = "CreatedInternetEgressNetworkPolicies"
)
