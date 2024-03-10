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
	"flag"
	"fmt"
	"github.com/bombsimon/logrusr/v3"
	certmanager "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/google/uuid"
	sa_pod_webhook_aws "github.com/otterize/credentials-operator/src/controllers/aws_iam/sa_pod_webhook"
	"github.com/otterize/credentials-operator/src/controllers/aws_iam/serviceaccount_old"
	"github.com/otterize/credentials-operator/src/controllers/aws_iam/spiffe_rolesanywhere_pod_webhook"
	"github.com/otterize/credentials-operator/src/controllers/certificates/otterizecertgen"
	"github.com/otterize/credentials-operator/src/controllers/certificates/spirecertgen"
	"github.com/otterize/credentials-operator/src/controllers/certmanageradapter"
	"github.com/otterize/credentials-operator/src/controllers/iam"
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
	mutatingwebhookconfiguration "github.com/otterize/intents-operator/src/operator/controllers/mutating_webhook_controller"
	operatorwebhooks "github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/azureagent"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/filters"
	"github.com/otterize/intents-operator/src/shared/gcpagent"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver"
	"github.com/otterize/intents-operator/src/shared/telemetries/componentinfo"
	"github.com/otterize/intents-operator/src/shared/telemetries/errorreporter"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetriesgql"
	"github.com/otterize/intents-operator/src/shared/telemetries/telemetrysender"
	"github.com/otterize/intents-operator/src/shared/version"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
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

	utilruntime.Must(gcpiamv1.AddToScheme(scheme))
	utilruntime.Must(gcpk8sv1.AddToScheme(scheme))

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
	errorreporter.Init("credentials-operator", version.Version(), viper.GetString(operatorconfig.TelemetryErrorsAPIKeyKey))
	defer errorreporter.AutoNotify()

	var secretsManager tls_pod.SecretsManager
	var workloadRegistry tls_pod.WorkloadRegistry
	var enableGCPServiceAccountManagement bool
	var enableAzureServiceAccountManagement bool
	var azureSubscriptionId string
	var azureResourceGroup string
	var azureAKSClusterName string
	flag.BoolVar(&enableGCPServiceAccountManagement, "enable-gcp-serviceaccount-management", false, "Create and bind ServiceAccounts to GCP IAM roles")
	flag.BoolVar(&enableAzureServiceAccountManagement, "enable-azure-serviceaccount-management", false, "Create and bind ServiceAccounts to Azure IAM roles")
	flag.StringVar(&azureSubscriptionId, "azure-subscription-id", "", "Azure subscription ID")
	flag.StringVar(&azureResourceGroup, "azure-resource-group", "", "Azure resource group")
	flag.StringVar(&azureAKSClusterName, "azure-aks-cluster-name", "", "Azure AKS cluster name")
	var userAndPassAcquirer poduserpassword.CloudUserAndPasswordAcquirer

	if viper.GetBool(operatorconfig.DebugKey) {
		logrus.SetLevel(logrus.DebugLevel)
	}
	logrus.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})

	ctrl.SetLogger(logrusr.New(logrus.StandardLogger()))

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
		logrus.WithError(err).Panic("unable to start manager")
	}

	signalHandlerCtx := ctrl.SetupSignalHandler()
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podNamespace == "" {
		logrus.Panic("POD_NAMESPACE environment variable is required")
	}

	kubeSystemUID := getClusterContextId(signalHandlerCtx, mgr)
	componentinfo.SetGlobalContextId(telemetrysender.Anonymize(kubeSystemUID))
	componentinfo.SetGlobalVersion(version.Version())

	serviceIdResolver := serviceidresolver.NewResolver(mgr.GetClient())
	eventRecorder := mgr.GetEventRecorderFor("credentials-operator")

	otterizeCloudClient, clientInitializedWithCredentials, err := otterizeclient.NewCloudClient(signalHandlerCtx)
	if err != nil {
		logrus.WithError(err).Panic("failed to initialize cloud client")
	}

	if clientInitializedWithCredentials {
		otterizeclient.PeriodicallyReportConnectionToCloud(otterizeCloudClient)
		userAndPassAcquirer = otterizeCloudClient
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

	iamAgents := make([]iam.IAMCredentialsAgent, 0)

	var awsAgent *awsagent.Agent

	if viper.GetBool(operatorconfig.EnableAWSServiceAccountManagementKey) {
		awsOptions := make([]awsagent.Option, 0)
		if viper.GetBool(operatorconfig.EnableAWSRolesAnywhereKey) {
			trustAnchorArn := viper.GetString(operatorconfig.AWSRolesAnywhereTrustAnchorARNKey)
			trustDomain := viper.GetString(operatorconfig.AWSRolesAnywhereSPIFFETrustDomainKey)
			clusterName := viper.GetString(operatorconfig.AWSRolesAnywhereClusterName)

			awsOptions = append(awsOptions, awsagent.WithRolesAnywhere(trustAnchorArn, trustDomain, clusterName))

			if viper.GetBool(operatorconfig.AWSUseSoftDeleteStrategyKey) {
				awsOptions = append(awsOptions, awsagent.WithSoftDeleteStrategy())
			}
		}
		awsAgent, err = awsagent.NewAWSAgent(signalHandlerCtx, awsOptions...)
		if err != nil {
			logrus.WithError(err).Panic("failed to initialize AWS agent")
		}
		awsIAMServiceAccountReconciler := serviceaccount_old.NewServiceAccountReconciler(client, awsAgent, viper.GetBool(operatorconfig.AWSUseSoftDeleteStrategyKey))
		if err = awsIAMServiceAccountReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithField("controller", "ServiceAccount").WithError(err).Panic("unable to create controller")
		}
	}

	if enableGCPServiceAccountManagement {
		gcpAgent, err := gcpagent.NewGCPAgent(signalHandlerCtx, client)
		if err != nil {
			logrus.WithError(err).Panic("failed to initialize GCP agent")
		}

		iamAgents = append(iamAgents, gcpAgent)
	}

	if enableAzureServiceAccountManagement {
		config := azureagent.Config{
			SubscriptionID: azureSubscriptionId,
			ResourceGroup:  azureResourceGroup,
			AKSClusterName: azureAKSClusterName,
		}

		azureAgent, err := azureagent.NewAzureAgent(signalHandlerCtx, config)
		if err != nil {
			logrus.WithError(err).Panic("failed to initialize Azure agent")
		}
		iamAgents = append(iamAgents, azureAgent)
	}

	azureGCPserviceAccountReconciler := serviceaccounts.NewServiceAccountReconciler(client, iamAgents)
	if err = azureGCPserviceAccountReconciler.SetupWithManager(mgr); err != nil {
		logrus.WithField("controller", "ServiceAccount").WithError(err).Panic("unable to create controller")
	}

	if enableAzureServiceAccountManagement || enableGCPServiceAccountManagement || viper.GetBool(operatorconfig.EnableAWSServiceAccountManagementKey) {
		podReconciler := pods.NewPodReconciler(client)
		if err = podReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithField("controller", "Pod").WithError(err).Panic("unable to create controller")
		}
	}

	if viper.GetBool(operatorconfig.SelfSignedCertKey) {
		var webhookHandler admission.Handler = sa_pod_webhook_generic.NewServiceAccountAnnotatingPodWebhook(mgr, iamAgents)
		logrus.Infoln("Creating self signing certs")
		certBundle, err :=
			operatorwebhooks.GenerateSelfSignedCertificate("credentials-operator-webhook-service", podNamespace)
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

		if viper.GetBool(operatorconfig.EnableAWSServiceAccountManagementKey) {
			webhookHandler = sa_pod_webhook_aws.NewServiceAccountAnnotatingPodWebhook(mgr, awsAgent)
		}

		if viper.GetBool(operatorconfig.EnableAWSRolesAnywhereKey) {
			webhookHandler = spiffe_rolesanywhere_pod_webhook.NewSPIFFEAWSRolePodWebhook(mgr, awsAgent, viper.GetString(operatorconfig.AWSRolesAnywhereTrustAnchorARNKey))
		}

		mgr.GetWebhookServer().Register("/mutate-v1-pod", &webhook.Admission{Handler: webhookHandler})

	}

	if viper.GetString(operatorconfig.CertProviderKey) != operatorconfig.CertProviderNone {
		certPodReconciler := tls_pod.NewCertificatePodReconciler(client, mgr.GetScheme(), workloadRegistry, secretsManager,
			serviceIdResolver, eventRecorder, viper.GetString(operatorconfig.CertProviderKey) == operatorconfig.CertProviderCloud)

		if err = certPodReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithField("controller", "certPod").WithError(err).Panic("unable to create controller")
		}
		go certPodReconciler.MaintenanceLoop(signalHandlerCtx)
	}

	if userAndPassAcquirer != nil {
		podUserAndPasswordReconciler := poduserpassword.NewReconciler(client, scheme, eventRecorder, serviceIdResolver, userAndPassAcquirer)
		if err = podUserAndPasswordReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithField("controller", "podUserAndPassword").WithError(err).Panic("unable to create controller")
		}
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

func getClusterContextId(ctx context.Context, mgr ctrl.Manager) string {
	metadataClient, err := metadata.NewForConfig(ctrl.GetConfigOrDie())
	if err != nil {
		logrus.WithError(err).Fatal("unable to create metadata client")
	}
	mapping, err := mgr.GetRESTMapper().RESTMapping(schema.GroupKind{Group: "", Kind: "Namespace"}, "v1")
	if err != nil {
		logrus.WithError(err).Fatal("unable to create Kubernetes API REST mapping")
	}
	kubeSystemUID := ""
	kubeSystemNs, err := metadataClient.Resource(mapping.Resource).Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil || kubeSystemNs == nil {
		logrus.WithError(err).Warningf("failed getting kubesystem UID: %s", err)
		return fmt.Sprintf("rand-%s", uuid.New().String())
	}
	kubeSystemUID = string(kubeSystemNs.UID)

	return kubeSystemUID
}
