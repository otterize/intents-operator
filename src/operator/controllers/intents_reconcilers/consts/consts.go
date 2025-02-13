package consts

// Consts have to go here to prevent import cycle between istiopolicy and intents_reconcilers.
const (
	ReasonEnforcementDefaultOff                                   = "EnforcementGloballyDisabled"
	ReasonNetworkPolicyCreationDisabled                           = "NetworkPolicyCreationDisabled"
	ReasonGettingNetworkPolicyFailed                              = "GettingNetworkPolicyFailed"
	ReasonRemovingNetworkPolicyFailed                             = "RemovingNetworkPolicyFailed"
	ReasonReconcilingNetworkPolicyFailed                          = "ReconcilingNetworkPolicyFailed"
	ReasonNamespaceNotAllowed                                     = "NamespaceNotAllowed"
	ReasonCreatingNetworkPoliciesFailed                           = "CreatingNetworkPoliciesFailed"
	ReasonCreatedNetworkPolicies                                  = "CreatedNetworkPolicies"
	ReasonIstioPolicyCreationDisabled                             = "IstioPolicyCreationDisabled"
	ReasonRemovingIstioPolicyFailed                               = "RemovingIstioPolicyFailed"
	ReasonPodsNotFound                                            = "PodsNotFound"
	ReasonIntentsFoundButNoServiceAccount                         = "ReasonIntentsFoundButNoServiceAccount"
	ReasonReconciledAWSPolicies                                   = "ReasonReconciledAWSPolicies"
	ReasonReconcilingAWSPoliciesFailed                            = "ReasonReconcilingAWSPoliciesFailed"
	ReasonReconciledIAMPolicies                                   = "ReasonReconciledIAMPolicies"
	ReasonReconcilingIAMPoliciesFailed                            = "ReasonReconcilingIAMPoliciesFailed"
	ReasonIntentsServiceAccountUsedByMultipleClients              = "ReasonIntentsServiceAccountUsedByMultipleClients"
	ReasonKubernetesServiceNotFound                               = "KubernetesServiceNotFound"
	ReasonPortRestrictionUnsupportedForStrings                    = "TypeStringPortNotSupported"
	ReasonEgressNetworkPolicyCreationDisabled                     = "EgressNetworkPolicyCreationDisabled"
	ReasonGettingEgressNetworkPolicyFailed                        = "GettingEgressNetworkPolicyFailed"
	ReasonRemovingEgressNetworkPolicyFailed                       = "RemovingEgressNetworkPolicyFailed"
	ReasonCreatingEgressNetworkPoliciesFailed                     = "CreatingEgressNetworkPoliciesFailed"
	ReasonCreatedEgressNetworkPolicies                            = "CreatedEgressNetworkPolicies"
	ReasonCreatedInternetEgressNetworkPolicies                    = "CreatedInternetEgressNetworkPolicies"
	ReasonIntentToUnresolvedDns                                   = "IntentToUnresolvedDns"
	ReasonInternetEgressNetworkPolicyCreationWaitingUnresolvedDNS = "InternetEgressNetworkPolicyCreationWaitingUnresolvedDNS"
	ReasonInternetEgressNetworkPolicyWithEgressPolicyDisabled     = "ReasonInternetEgressNetworkPolicyWithEgressPolicyDisabled"
)
