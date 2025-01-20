package operator_cloud_client

import (
	"context"
	"github.com/otterize/intents-operator/src/operator/controllers/istiopolicy"
	linkerdmanager "github.com/otterize/intents-operator/src/operator/controllers/linkerd"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/allowexternaltraffic"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/enforcement"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

func StartPeriodicCloudReports(ctx context.Context, client CloudClient, kubeClient client.Client) {
	statusReportInterval := viper.GetInt(otterizecloudclient.ComponentReportIntervalKey)
	configReportInterval := viper.GetInt(otterizecloudclient.OperatorConfigReportIntervalKey)

	go func() {
		defer errorreporter.AutoNotify()
		runPeriodicReportConnection(ctx, statusReportInterval, configReportInterval, client, kubeClient)
	}()
}

func runPeriodicReportConnection(ctx context.Context, statusInterval int, configReportInterval int, client CloudClient, kubeClient client.Client) {
	cloudUploadTicker := time.NewTicker(time.Second * time.Duration(statusInterval))
	configUploadTicker := time.NewTicker(time.Second * time.Duration(configReportInterval))

	logrus.Info("Starting cloud connection ticker")
	reportStatus(ctx, client)
	uploadConfiguration(ctx, client, kubeClient)

	for {
		select {
		case <-cloudUploadTicker.C:
			reportStatus(ctx, client)

		case <-configUploadTicker.C:
			uploadConfiguration(ctx, client, kubeClient)

		case <-ctx.Done():
			logrus.Info("Periodic upload exit")
			return
		}
	}
}

func reportStatus(ctx context.Context, client CloudClient) {
	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	client.ReportComponentStatus(timeoutCtx, graphqlclient.ComponentTypeIntentsOperator)
}

func getAllowExternalConfig() graphqlclient.AllowExternalTrafficPolicy {
	switch enforcement.GetConfig().AllowExternalTraffic {
	case allowexternaltraffic.Always:
		return graphqlclient.AllowExternalTrafficPolicyAlways
	case allowexternaltraffic.Off:
		return graphqlclient.AllowExternalTrafficPolicyOff
	case allowexternaltraffic.IfBlockedByOtterize:
		return graphqlclient.AllowExternalTrafficPolicyIfBlockedByOtterize
	default:
		return ""
	}
}

func uploadConfiguration(ctx context.Context, client CloudClient, kubeClient client.Client) {
	ingressConfigIdentities := operatorconfig.GetIngressControllerServiceIdentities()
	externallyManagedPolicyWorkloadIdentities := operatorconfig.GetExternallyManagedPoliciesServiceIdentities()
	enforcementConfig := enforcement.GetConfig()
	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	// This has to be checked here rather in the enforcement config, because the enforcement config will not be updated if Istio is installed after the fact
	isIstioInstalled, err := istiopolicy.IsIstioAuthorizationPoliciesInstalled(ctx, kubeClient)
	if err != nil {
		logrus.WithError(err).Error("Failed to check if Istio CRDs are installed, assuming yes")
		isIstioInstalled = true
		// Intentionally no return here, as we want to report the configuration even if the check failed
	}

	isLinkerdInstalled, err := linkerdmanager.IsLinkerdInstalled(ctx, kubeClient)
	if err != nil {
		logrus.WithError(err).Error("Failed to check if Linkerd CRDs exist, assuming yes")
		isLinkerdInstalled = true
	}

	configInput := graphqlclient.IntentsOperatorConfigurationInput{
		GlobalEnforcementEnabled:              enforcementConfig.EnforcementDefaultState,
		NetworkPolicyEnforcementEnabled:       enforcementConfig.EnableNetworkPolicy,
		EgressNetworkPolicyEnforcementEnabled: enforcementConfig.EnableEgressNetworkPolicyReconcilers,
		KafkaACLEnforcementEnabled:            enforcementConfig.EnableKafkaACL,
		AwsIAMPolicyEnforcementEnabled:        enforcementConfig.EnableAWSPolicy,
		GcpIAMPolicyEnforcementEnabled:        enforcementConfig.EnableGCPPolicy,
		AzureIAMPolicyEnforcementEnabled:      enforcementConfig.EnableAzurePolicy,
		DatabaseEnforcementEnabled:            enforcementConfig.EnableDatabasePolicy,
		IstioPolicyEnforcementEnabled:         enforcementConfig.EnableIstioPolicy && isIstioInstalled,
		LinkerdPolicyEnforcementEnabled:       enforcementConfig.EnableLinkerdPolicies && isLinkerdInstalled,
		ProtectedServicesEnabled:              enforcementConfig.EnableNetworkPolicy, // in this version, protected services are enabled if network policy creation is enabled, regardless of enforcement default state
		EnforcedNamespaces:                    enforcementConfig.EnforcedNamespaces.Items(),
		AllowExternalTrafficPolicy:            getAllowExternalConfig(),
	}

	configInput.IngressControllerConfig = lo.Map(ingressConfigIdentities, func(identity serviceidentity.ServiceIdentity, _ int) graphqlclient.IngressControllerConfigInput {
		return graphqlclient.IngressControllerConfigInput{
			Name:      identity.Name,
			Namespace: identity.Namespace,
			Kind:      identity.Kind,
		}
	})

	configInput.ExternallyManagedPolicyWorkloads = lo.Map(externallyManagedPolicyWorkloadIdentities, func(identity serviceidentity.ServiceIdentity, _ int) graphqlclient.ExternallyManagedPolicyWorkloadInput {
		return graphqlclient.ExternallyManagedPolicyWorkloadInput{
			Name:      identity.Name,
			Namespace: identity.Namespace,
			Kind:      identity.Kind,
		}
	})

	configInput.AwsALBLoadBalancerExemptionEnabled = viper.GetBool(operatorconfig.IngressControllerALBExemptKey)

	client.ReportIntentsOperatorConfiguration(timeoutCtx, configInput)
}
