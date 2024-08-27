package operator_cloud_client

import (
	"context"
	"github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/operatorconfig/enforcement"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"time"
)

func StartPeriodicallyReportConnectionToCloud(client CloudClient, ctx context.Context) {
	statusReportInterval := viper.GetInt(otterizecloudclient.ComponentReportIntervalKey)
	configReportInterval := viper.GetInt(otterizecloudclient.OperatorConfigReportIntervalKey)

	go func() {
		defer errorreporter.AutoNotify()
		runPeriodicReportConnection(statusReportInterval, configReportInterval, client, ctx)
	}()
}

func runPeriodicReportConnection(statusInterval int, configReportInterval int, client CloudClient, ctx context.Context) {
	cloudUploadTicker := time.NewTicker(time.Second * time.Duration(statusInterval))
	configUploadTicker := time.NewTicker(time.Second * time.Duration(configReportInterval))

	logrus.Info("Starting cloud connection ticker")
	reportStatus(ctx, client)
	uploadConfiguration(ctx, client)

	for {
		select {
		case <-cloudUploadTicker.C:
			reportStatus(ctx, client)

		case <-configUploadTicker.C:
			uploadConfiguration(ctx, client)

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

func uploadConfiguration(ctx context.Context, client CloudClient) {
	ingressConfigIdentities := operatorconfig.GetIngressControllerServiceIdentities()
	enforcementConfig := enforcement.GetConfig()
	timeoutCtx, cancel := context.WithTimeout(ctx, viper.GetDuration(otterizecloudclient.CloudClientTimeoutKey))
	defer cancel()

	configInput := graphqlclient.IntentsOperatorConfigurationInput{
		GlobalEnforcementEnabled:              enforcementConfig.EnforcementDefaultState,
		NetworkPolicyEnforcementEnabled:       enforcementConfig.EnableNetworkPolicy,
		EgressNetworkPolicyEnforcementEnabled: enforcementConfig.EnableEgressNetworkPolicyReconcilers,
		KafkaACLEnforcementEnabled:            enforcementConfig.EnableKafkaACL,
		AwsIAMPolicyEnforcementEnabled:        enforcementConfig.EnableAWSPolicy,
		GcpIAMPolicyEnforcementEnabled:        enforcementConfig.EnableGCPPolicy,
		AzureIAMPolicyEnforcementEnabled:      enforcementConfig.EnableAzurePolicy,
		DatabaseEnforcementEnabled:            enforcementConfig.EnableDatabasePolicy,
		IstioPolicyEnforcementEnabled:         enforcementConfig.EnableIstioPolicy,
		ProtectedServicesEnabled:              enforcementConfig.EnableNetworkPolicy, // in this version, protected services are enabled if network policy creation is enabled, regardless of enforcement default state
		EnforcedNamespaces:                    enforcementConfig.EnforcedNamespaces.Items(),
	}

	if len(ingressConfigIdentities) != 0 {
		ingressControllerConfigInput := make([]graphqlclient.IngressControllerConfigInput, 0)
		for _, identity := range ingressConfigIdentities {
			ingressControllerConfigInput = append(ingressControllerConfigInput, graphqlclient.IngressControllerConfigInput{
				Name:      identity.Name,
				Namespace: identity.Namespace,
				Kind:      identity.Kind,
			})
		}
		configInput.IngressControllerConfig = ingressControllerConfigInput
	}

	configInput.AwsALBLoadBalancerExemptionEnabled = viper.GetBool(operatorconfig.IngressControllerALBExemptKey)

	client.ReportIntentsOperatorConfiguration(timeoutCtx, configInput)
}
