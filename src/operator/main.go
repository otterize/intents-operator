/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"github.com/bombsimon/logrusr/v3"
	certmanager "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/otterize/credentials-operator/src/controllers/certificates/otterizecertgen"
	"github.com/otterize/credentials-operator/src/controllers/certificates/spirecertgen"
	"github.com/otterize/credentials-operator/src/controllers/certmanageradapter"
	"github.com/otterize/credentials-operator/src/controllers/iam/iamcredentialsagents"
	"github.com/otterize/credentials-operator/src/controllers/iam/iamcredentialsagents/awscredentialsagent"
	"github.com/otterize/credentials-operator/src/controllers/iam/iamcredentialsagents/azurecredentialsagent"
	"github.com/otterize/credentials-operator/src/controllers/iam/iamcredentialsagents/gcpcredentialsagent"
	"github.com/otterize/credentials-operator/src/controllers/iam/pods"
	"github.com/otterize/credentials-operator/src/controllers/iam/serviceaccounts"
	sa_pod_webhook_generic "github.com/otterize/credentials-operator/src/controllers/iam/webhooks"
	"github.com/otterize/credentials-operator/src/controllers/otterizeclient"
	"github.com/otterize/credentials-operator/src/controllers/poduserpassword"
	"github.com/otterize/credentials-operator/src/controllers/secrets"
	"github.com/otterize/credentials-operator/src/controllers/spireclient"
	"github.com/otterize/credentials-operator/src/controllers/spireclient/bundles"
	"github.com/otterize/credentials-operator/src/controllers/spireclient/entries"
	"github.com/otterize/credentials-operator/src/controllers/spireclient/svids"
	"github.com/otterize/credentials-operator/src/controllers/tls_pod"
	"github.com/otterize/credentials-operator/src/operatorconfig"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	otterizev1beta1 "github.com/otterize/intents-operator/src/operator/api/v1beta1"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	mutatingwebhookconfiguration "github.com/otterize/intents-operator/src/operator/controllers/mutating_webhook_controller"
	"github.com/otterize/intents-operator/src/operator/otterizecrds"
	operatorwebhooks "github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/otterize/intents-operator/src/shared"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	"github.com/otterize/intents-operator/src/shared/clusterutils"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/filters"
	"github.com/otterize/intents-operator/src/shared/gcpagent"
	sharedconfig "github.com/otterize/intents-operator/src/shared/operatorconfig"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/telemetries/componentinfo"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/otterize/intents-operator/src/shared/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"os"
	"path"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	gcpiamv1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/iam/v1beta1"
	gcpk8sv1 "github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/k8s/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	// +kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()
)

const (
	socketPath = "unix:////run/spire/sockets/agent.sock"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(certmanager.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(gcpiamv1.AddToScheme(scheme))
	utilruntime.Must(gcpk8sv1.AddToScheme(scheme))
	utilruntime.Must(otterizev1alpha3.AddToScheme(scheme))
	utilruntime.Must(otterizev1beta1.AddToScheme(scheme))
	utilruntime.Must(otterizev2alpha1.AddToScheme(scheme))

	// +kubebuilder:scaffold:scheme
}

func initSpireClient(ctx context.Context, spireServerAddr string) (spireclient.ServerClient, error) {
	// fetch Certificate & bundle through spire-agent API
	source, err := workloadapi.NewX509Source(ctx, workloadapi.WithClientOptions(workloadapi.WithAddr(socketPath), workloadapi.WithLogger(logrus.StandardLogger())))
	if err != nil {
		return nil, errors.Wrap(err)
	}

	serverClient, err := spireclient.NewServerClient(ctx, spireServerAddr, source)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	logrus.WithField("server_address", spireServerAddr).Infof("Successfully connected to SPIRE server")
	return serverClient, nil
}

func main() {
	var secretsManager tls_pod.SecretsManager
	var workloadRegistry tls_pod.WorkloadRegistry

	if viper.GetBool(operatorconfig.DebugKey) {
		logrus.SetLevel(logrus.DebugLevel)
	}
	logrus.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})

	signalHandlerCtx := ctrl.SetupSignalHandler()

	clusterUID := clusterutils.GetOrCreateClusterUID(signalHandlerCtx)

	componentinfo.SetGlobalContextId(telemetrysender.Anonymize(clusterUID))

	ctrl.SetLogger(logrusr.New(logrus.StandardLogger()))

	errorreporter.Init(telemetriesgql.TelemetryComponentTypeCredentialsOperator, version.Version())
	defer errorreporter.AutoNotify()
	shared.RegisterPanicHandlers()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: viper.GetString(operatorconfig.MetricsAddrKey),
		},
		WebhookServer:          webhook.NewServer(webhook.Options{Port: 9443, CertDir: operatorwebhooks.CertDirPath}),
		HealthProbeBindAddress: viper.GetString(operatorconfig.ProbeAddrKey),
		LeaderElection:         viper.GetBool(operatorconfig.EnableLeaderElectionKey),
		LeaderElectionID:       "credentials-operator.otterize.com",
	})
	if err != nil {
		logrus.WithError(err).Panic("unable to initialize manager")
	}

	podNamespace := os.Getenv("POD_NAMESPACE")
	if podNamespace == "" {
		logrus.Panic("POD_NAMESPACE environment variable is required")
	}

	directClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		logrus.WithError(err).Panic("unable to create kubernetes API client")
	}

	// Required for cases where the intents operator starts after the credentials operator, and the credentials
	// operator requires ClientIntents to be deployed.
	err = otterizecrds.Ensure(signalHandlerCtx, directClient, podNamespace, []byte{})
	if err != nil {
		logrus.WithError(err).Panic("unable to ensure otterize CRDs")
	}

	serviceIdResolver := serviceidresolver.NewResolver(mgr.GetClient())
	eventRecorder := mgr.GetEventRecorderFor("credentials-operator")

	otterizeCloudClient, clientInitializedWithCredentials, err := otterizeclient.NewCloudClient(signalHandlerCtx)
	if err != nil {
		logrus.WithError(err).Panic("failed to initialize cloud client")
	}

	if clientInitializedWithCredentials {
		otterizeclient.PeriodicallyReportConnectionToCloud(otterizeCloudClient)
	}

	if viper.GetString(operatorconfig.CertProviderKey) == operatorconfig.CertProviderCloud {
		if !clientInitializedWithCredentials {
			logrus.WithError(err).Panic("using cloud, but cloud credentials not specified")
		}
		workloadRegistry = otterizeCloudClient
		otterizeCertManager := otterizecertgen.NewOtterizeCertificateGenerator(otterizeCloudClient)
		secretsManager = secrets.NewDirectSecretsManager(mgr.GetClient(), serviceIdResolver, eventRecorder, otterizeCertManager)
	} else if viper.GetString(operatorconfig.CertProviderKey) == operatorconfig.CertProviderSPIRE {
		spireClient, err := initSpireClient(signalHandlerCtx, viper.GetString(operatorconfig.SpireServerAddrKey))
		if err != nil {
			logrus.WithError(err).Panic("failed to connect to spire server")
		}
		defer spireClient.Close()
		bundlesStore := bundles.NewBundlesStore(spireClient)
		svidsStore := svids.NewSVIDsStore(spireClient)
		certGenerator := spirecertgen.NewSpireCertificateDataGenerator(bundlesStore, svidsStore)
		secretsManager = secrets.NewDirectSecretsManager(mgr.GetClient(), serviceIdResolver, eventRecorder, certGenerator)
		workloadRegistry = entries.NewSpireRegistry(spireClient)
	} else if viper.GetString(operatorconfig.CertProviderKey) == operatorconfig.CertProviderCertManager {
		cmSecretsManager, cmWorkloadRegistry := certmanageradapter.NewCertManagerSecretsManager(mgr.GetClient(), serviceIdResolver,
			eventRecorder, viper.GetString(operatorconfig.CertManagerIssuerKey), viper.GetBool(operatorconfig.CertManagerUseClustierIssuerKey))

		if viper.GetBool(operatorconfig.UseCertManagerApproverKey) {
			if err = cmSecretsManager.RegisterCertificateApprover(signalHandlerCtx, mgr); err != nil {
				logrus.WithField("controller", "CertificateRequest").WithError(err).Panic("unable to create controller")
			}
		}

		secretsManager = cmSecretsManager
		workloadRegistry = cmWorkloadRegistry
	} else if viper.GetString(operatorconfig.CertProviderKey) == operatorconfig.CertProviderNone {
		// intentionally do nothing
	} else {
		logrus.WithField("provider", viper.GetString(operatorconfig.CertProviderKey)).Panic("unexpected value for provider")
	}

	client := mgr.GetClient()
	setupIAMAgents(signalHandlerCtx, mgr, client, podNamespace)

	// setup webhook
	if viper.GetBool(operatorconfig.SelfSignedCertKey) {
		logrus.Infoln("Creating self signing certs")
		certBundle, err := operatorwebhooks.GenerateSelfSignedCertificate("credentials-operator-webhook-service", podNamespace)
		if err != nil {
			logrus.WithError(err).Panic("unable to create self signed certs for webhook")
		}
		err = operatorwebhooks.WriteCertToFiles(certBundle)
		if err != nil {
			logrus.WithError(err).Panic("failed writing certs to file system")
		}

		//+kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=mutatingwebhookconfigurations,verbs=get;update;patch;list;watch
		reconciler := mutatingwebhookconfiguration.NewMutatingWebhookConfigsReconciler(client, mgr.GetScheme(), certBundle.CertPem, filters.CredentialsOperatorLabelPredicate())
		if err = reconciler.SetupWithManager(mgr); err != nil {
			logrus.WithField("controller", "MutatingWebhookConfigs").WithError(err).Panic("unable to create controller")
		}
	}

	if viper.GetString(operatorconfig.CertProviderKey) != operatorconfig.CertProviderNone {
		certPodReconciler := tls_pod.NewCertificatePodReconciler(client, mgr.GetScheme(), workloadRegistry, secretsManager,
			serviceIdResolver, eventRecorder, viper.GetString(operatorconfig.CertProviderKey) == operatorconfig.CertProviderCloud)

		if err = certPodReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithField("controller", "certPod").WithError(err).Panic("unable to create controller")
		}
		go certPodReconciler.MaintenanceLoop(signalHandlerCtx)
	}

	podUserAndPasswordReconciler := poduserpassword.NewReconciler(client, scheme, eventRecorder, serviceIdResolver)
	if err = podUserAndPasswordReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithField("controller", "podUserAndPassword").WithError(err).Panic("unable to create controller")
	}
	if viper.GetBool(operatorconfig.EnableSecretRotationKey) {
		go podUserAndPasswordReconciler.RotateSecretsLoop(signalHandlerCtx)
	}

	// +kubebuilder:scaffold:builder
	if err := mgr.AddHealthzCheck("health", healthz.Ping); err != nil {
		logrus.WithError(err).Panic("unable to set up health check")
	}
	if err := mgr.AddReadyzCheck("check", healthz.Ping); err != nil {
		logrus.WithError(err).Panic("unable to set up ready check")
	}

	telemetrysender.SendCredentialsOperator(telemetriesgql.EventTypeStarted, 1)
	telemetrysender.CredentialsOperatorRunActiveReporter(signalHandlerCtx)
	logrus.Info("starting manager")

	if err := mgr.Start(signalHandlerCtx); err != nil {
		logrus.WithError(err).Panic("problem running manager")
	}
}

func initAWSMultiaccountCredentialsAgent(ctx context.Context, agentOptions []awsagent.Option, accounts []sharedconfig.AWSAccount, clusterName string, keyPath string, certPath string) iamcredentialsagents.IAMCredentialsAgent {
	credentialsAgent, err := awscredentialsagent.NewMultiaccountAWSCredentialsAgent(ctx, accounts, agentOptions, clusterName, keyPath, certPath)
	if err != nil {
		logrus.WithError(err).Panic("failed to initialize AWS agent")
	}
	return credentialsAgent
}

func initAWSCredentialsAgent(ctx context.Context) iamcredentialsagents.IAMCredentialsAgent {
	clusterName := viper.GetString(operatorconfig.AWSRolesAnywhereClusterName)
	certPath := path.Join(viper.GetString(sharedconfig.AWSRolesAnywhereCertDirKey), viper.GetString(sharedconfig.AWSRolesAnywhereCertFilenameKey))
	keyPath := path.Join(viper.GetString(sharedconfig.AWSRolesAnywhereCertDirKey), viper.GetString(sharedconfig.AWSRolesAnywherePrivKeyFilenameKey))
	awsOptions := make([]awsagent.Option, 0)
	if viper.GetBool(operatorconfig.AWSUseSoftDeleteStrategyKey) {
		awsOptions = append(awsOptions, awsagent.WithSoftDeleteStrategy())
	}

	if viper.GetBool(operatorconfig.EnableAWSRolesAnywhereKey) {
		accounts := sharedconfig.GetRolesAnywhereAWSAccounts()
		if len(accounts) == 0 {
			logrus.Panic("no AWS accounts configured even though RolesAnywhere is enabled")
		}
		if len(accounts) == 1 {
			awsOptions = append(awsOptions, awsagent.WithRolesAnywhere(accounts[0], clusterName, keyPath, certPath))
		} else {
			return initAWSMultiaccountCredentialsAgent(ctx, awsOptions, accounts, clusterName, keyPath, certPath)
		}

	}

	awsAgent, err := awsagent.NewAWSAgent(ctx, awsOptions...)
	if err != nil {
		logrus.WithError(err).Panic("failed to initialize AWS agent")
	}

	return awscredentialsagent.NewAWSCredentialsAgent(awsAgent)
}

func initGCPCredentialsAgent(ctx context.Context, client controllerruntimeclient.Client) *gcpcredentialsagent.Agent {
	gcpAgent, err := gcpagent.NewGCPAgent(ctx, client)
	if err != nil {
		logrus.WithError(err).Panic("failed to initialize GCP agent")
	}

	return gcpcredentialsagent.NewGCPCredentialsAgent(gcpAgent)
}

func initAzureCredentialsAgent(ctx context.Context) *azurecredentialsagent.Agent {
	config := azureagent.Config{
		SubscriptionID: viper.GetString(operatorconfig.AzureSubscriptionIdKey),
		ResourceGroup:  viper.GetString(operatorconfig.AzureResourceGroupKey),
		AKSClusterName: viper.GetString(operatorconfig.AzureAksClusterNameKey),
	}

	azureAgent, err := azureagent.NewAzureAgent(ctx, config)
	if err != nil {
		logrus.WithError(err).Panic("failed to initialize Azure agent")
	}
	return azurecredentialsagent.NewAzureCredentialsAgent(azureAgent)
}

func setupIAMAgents(ctx context.Context, mgr ctrl.Manager, client controllerruntimeclient.Client, podNamespace string) {
	awsIAMEnabled := viper.GetBool(operatorconfig.EnableAWSServiceAccountManagementKey)
	azureIAMEnabled := viper.GetBool(operatorconfig.EnableAzureServiceAccountManagementKey)
	gcpIAMEnabled := viper.GetBool(operatorconfig.EnableGCPServiceAccountManagementKey)

	disableWebhookServer := viper.GetBool(operatorconfig.DisableWebhookServerKey)

	if !awsIAMEnabled && !azureIAMEnabled && !gcpIAMEnabled {
		return
	}

	iamAgents := make([]iamcredentialsagents.IAMCredentialsAgent, 0)

	if awsIAMEnabled {
		awsCredentialsAgent := initAWSCredentialsAgent(ctx)
		iamAgents = append(iamAgents, awsCredentialsAgent)

		if !disableWebhookServer {
			awsWebhookHandler := sa_pod_webhook_generic.NewServiceAccountAnnotatingPodWebhook(mgr, awsCredentialsAgent)
			mgr.GetWebhookServer().Register("/mutate-aws-v1-pod", &webhook.Admission{Handler: awsWebhookHandler})
		}
	}

	if gcpIAMEnabled {
		gcpCredentialsAgent := initGCPCredentialsAgent(ctx, client)
		iamAgents = append(iamAgents, gcpCredentialsAgent)
	}

	if azureIAMEnabled {
		azureCredentialsAgent := initAzureCredentialsAgent(ctx)
		iamAgents = append(iamAgents, azureCredentialsAgent)

		if !disableWebhookServer {
			azureWebhookHandler := sa_pod_webhook_generic.NewServiceAccountAnnotatingPodWebhook(mgr, azureCredentialsAgent)
			mgr.GetWebhookServer().Register("/mutate-azure-v1-pod", &webhook.Admission{Handler: azureWebhookHandler})
		}
	}

	// setup service account reconciler
	for _, iamAgent := range iamAgents {
		// setup pod reconciler
		podReconciler := pods.NewPodReconciler(client, iamAgent)
		if err := podReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithField("controller", "Pod").WithError(err).Panic("unable to create controller")
		}

		serviceAccountReconciler := serviceaccounts.NewServiceAccountReconciler(client, iamAgent)
		if err := serviceAccountReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithField("controller", "ServiceAccount").WithError(err).Panic("unable to create controller")
		}
	}
}
