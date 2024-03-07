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
	"github.com/otterize/credentials-operator/src/controllers/iam/pods"
	"github.com/otterize/intents-operator/src/shared/azureagent"

	"github.com/otterize/credentials-operator/src/controllers/certificates/otterizecertgen"
	"github.com/otterize/credentials-operator/src/controllers/certificates/spirecertgen"
	"github.com/otterize/credentials-operator/src/controllers/certmanageradapter"
	"github.com/otterize/credentials-operator/src/controllers/iam"
	serviceaccount "github.com/otterize/credentials-operator/src/controllers/iam/serviceaccounts"
	"github.com/otterize/credentials-operator/src/controllers/iam/webhooks"
	"github.com/otterize/credentials-operator/src/controllers/otterizeclient"
	"github.com/otterize/credentials-operator/src/controllers/poduserpassword"
	"github.com/otterize/credentials-operator/src/controllers/secrets"
	"github.com/otterize/credentials-operator/src/controllers/spireclient"
	"github.com/otterize/credentials-operator/src/controllers/spireclient/bundles"
	"github.com/otterize/credentials-operator/src/controllers/spireclient/entries"
	"github.com/otterize/credentials-operator/src/controllers/spireclient/svids"
	"github.com/otterize/credentials-operator/src/controllers/tls_pod"
	"github.com/otterize/credentials-operator/src/operatorconfig"
	operatorwebhooks "github.com/otterize/intents-operator/src/operator/webhooks"
	"github.com/otterize/intents-operator/src/shared/awsagent"
	"github.com/otterize/intents-operator/src/shared/errors"
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
	"golang.org/x/exp/slices"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"strings"
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

const (
	CertProviderSPIRE       CertProvider = "spire"
	CertProviderCloud       CertProvider = "otterize-cloud"
	CertProviderCertManager CertProvider = "cert-manager"
	CertProviderNone        CertProvider = "none"
)

type CertProvider string

func (cpf *CertProvider) getOptionalValues() []string {
	return []string{string(CertProviderSPIRE), string(CertProviderCloud), string(CertProviderCertManager), string(CertProviderNone)}
}

func (cpf *CertProvider) GetPrintableOptionalValues() string {
	return strings.Join(cpf.getOptionalValues(), ", ")
}

func (cpf *CertProvider) Set(v string) error {
	if slices.Contains(cpf.getOptionalValues(), v) {
		*cpf = CertProvider(v)
		return nil
	}
	return fmt.Errorf("certificate-provider should be one of: %s", cpf.GetPrintableOptionalValues())
}

func (cpf *CertProvider) String() string {
	return string(*cpf)
}

func main() {
	errorreporter.Init("credentials-operator", version.Version(), viper.GetString(operatorconfig.TelemetryErrorsAPIKeyKey))
	defer errorreporter.AutoNotify()

	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var spireServerAddr string
	certProvider := CertProviderNone
	var certManagerIssuer string
	var selfSignedCert bool
	var certManagerUseClusterIssuer bool
	var useCertManagerApprover bool
	var secretsManager tls_pod.SecretsManager
	var workloadRegistry tls_pod.WorkloadRegistry
	var enableAWSServiceAccountManagement bool
	var awsUseSoftDeleteStrategy bool
	var enableGCPServiceAccountManagement bool
	var enableAzureServiceAccountManagement bool
	var azureSubscriptionId string
	var azureResourceGroup string
	var azureAKSClusterName string
	var debug bool
	var userAndPassAcquirer poduserpassword.CloudUserAndPasswordAcquirer
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":7071", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":7072", "The address the probe endpoint binds to.")
	flag.StringVar(&spireServerAddr, "spire-server-address", "spire-server.spire:8081", "SPIRE server API address.")
	flag.Var(&certProvider, "certificate-provider", fmt.Sprintf("Certificate generation provider (%s)", certProvider.GetPrintableOptionalValues()))
	flag.StringVar(&certManagerIssuer, "cert-manager-issuer", "ca-issuer", "Name of the Issuer to be used by cert-manager to sign certificates")
	flag.BoolVar(&selfSignedCert, "self-signed-cert", true, "Whether to generate and update a self-signed cert for Webhooks")
	flag.BoolVar(&certManagerUseClusterIssuer, "cert-manager-use-cluster-issuer", false, "Use ClusterIssuer instead of a (namespace bound) Issuer")
	flag.BoolVar(&useCertManagerApprover, "cert-manager-approve-requests", false, "Make credentials-operator approve its own CertificateRequests")
	flag.BoolVar(&enableAWSServiceAccountManagement, "enable-aws-serviceaccount-management", false, "Create and bind ServiceAccounts to AWS IAM roles")
	flag.BoolVar(&awsUseSoftDeleteStrategy, "aws-use-soft-delete", false, "Mark AWS roles and policies as deleted instead of actually deleting them")
	flag.BoolVar(&enableGCPServiceAccountManagement, "enable-gcp-serviceaccount-management", false, "Create and bind ServiceAccounts to GCP IAM roles")
	flag.BoolVar(&enableAzureServiceAccountManagement, "enable-azure-serviceaccount-management", false, "Create and bind ServiceAccounts to Azure IAM roles")
	flag.StringVar(&azureSubscriptionId, "azure-subscription-id", "", "Azure subscription ID")
	flag.StringVar(&azureResourceGroup, "azure-resource-group", "", "Azure resource group")
	flag.StringVar(&azureAKSClusterName, "azure-aks-cluster-name", "", "Azure AKS cluster name")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging")

	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	logrus.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: time.RFC3339,
	})

	ctrl.SetLogger(logrusr.New(logrus.StandardLogger()))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "credentials-operator.otterize.com",
	})
	if err != nil {
		logrus.WithError(err).Panic("unable to start manager")
	}

	ctx := ctrl.SetupSignalHandler()
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podNamespace == "" {
		logrus.Panic("POD_NAMESPACE environment variable is required")
	}

	kubeSystemUID := getClusterContextId(ctx, mgr)
	componentinfo.SetGlobalContextId(telemetrysender.Anonymize(kubeSystemUID))
	componentinfo.SetGlobalVersion(version.Version())

	serviceIdResolver := serviceidresolver.NewResolver(mgr.GetClient())
	eventRecorder := mgr.GetEventRecorderFor("credentials-operator")

	otterizeCloudClient, clientInitializedWithCredentials, err := otterizeclient.NewCloudClient(ctx)
	if err != nil {
		logrus.WithError(err).Panic("failed to initialize cloud client")
	}

	if clientInitializedWithCredentials {
		otterizeclient.PeriodicallyReportConnectionToCloud(otterizeCloudClient)
		userAndPassAcquirer = otterizeCloudClient
	}

	if certProvider == CertProviderCloud {
		if !clientInitializedWithCredentials {
			logrus.WithError(err).Panic("using cloud, but cloud credentials not specified")
		}
		workloadRegistry = otterizeCloudClient
		otterizeCertManager := otterizecertgen.NewOtterizeCertificateGenerator(otterizeCloudClient)
		secretsManager = secrets.NewDirectSecretsManager(mgr.GetClient(), serviceIdResolver, eventRecorder, otterizeCertManager)
	} else if certProvider == CertProviderSPIRE {
		spireClient, err := initSpireClient(ctx, spireServerAddr)
		if err != nil {
			logrus.WithError(err).Panic("failed to connect to spire server")
		}
		defer spireClient.Close()
		bundlesStore := bundles.NewBundlesStore(spireClient)
		svidsStore := svids.NewSVIDsStore(spireClient)
		certGenerator := spirecertgen.NewSpireCertificateDataGenerator(bundlesStore, svidsStore)
		secretsManager = secrets.NewDirectSecretsManager(mgr.GetClient(), serviceIdResolver, eventRecorder, certGenerator)
		workloadRegistry = entries.NewSpireRegistry(spireClient)
	} else if certProvider == CertProviderCertManager {
		secretsManager, workloadRegistry = certmanageradapter.NewCertManagerSecretsManager(mgr.GetClient(), serviceIdResolver,
			eventRecorder, certManagerIssuer, certManagerUseClusterIssuer)
	} else if certProvider == CertProviderNone {
		// intentionally do nothing
	} else {
		logrus.WithField("provider", certProvider).Panic("unexpected value for provider")
	}
	client := mgr.GetClient()

	iamAgents := make([]iam.IAMCredentialsAgent, 0)

	if enableAWSServiceAccountManagement {
		awsAgent, err := awsagent.NewAWSAgent(ctx)
		if err != nil {
			logrus.WithError(err).Panic("failed to initialize AWS agent")
		}

		awsAgent.SetSoftDeleteStrategy(awsUseSoftDeleteStrategy)
		iamAgents = append(iamAgents, awsAgent)
	}

	if enableGCPServiceAccountManagement {
		gcpAgent, err := gcpagent.NewGCPAgent(ctx, client)
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

		azureAgent, err := azureagent.NewAzureAgent(ctx, config)
		if err != nil {
			logrus.WithError(err).Panic("failed to initialize Azure agent")
		}
		iamAgents = append(iamAgents, azureAgent)
	}

	if len(iamAgents) > 0 {
		podReconciler := pods.NewPodReconciler(client)
		if err = podReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithField("controller", "PodReconciler").WithError(err).Panic("unable to create controller")
		}

		serviceAccountReconciler := serviceaccount.NewServiceAccountReconciler(client, iamAgents)
		if err = serviceAccountReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithField("controller", "ServiceAccount").WithError(err).Panic("unable to create controller")
		}

		if selfSignedCert {
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

			err = operatorwebhooks.UpdateMutationWebHookCA(context.Background(),
				"otterize-credentials-operator-mutating-webhook-configuration", certBundle.CertPem)
			if err != nil {
				logrus.WithError(err).Panic("updating validation webhook certificate failed")
			}

			podAnnotatorWebhook := webhooks.NewServiceAccountAnnotatingPodWebhook(mgr, iamAgents)
			mgr.GetWebhookServer().Register("/mutate-v1-pod", &webhook.Admission{Handler: podAnnotatorWebhook})
		}

	}

	if certProvider != CertProviderNone {
		certPodReconciler := tls_pod.NewCertificatePodReconciler(client, mgr.GetScheme(), workloadRegistry, secretsManager,
			serviceIdResolver, eventRecorder, certProvider == CertProviderCloud)

		if err = certPodReconciler.SetupWithManager(mgr); err != nil {
			logrus.WithField("controller", "certPod").WithError(err).Panic("unable to create controller")
		}
		go certPodReconciler.MaintenanceLoop(ctx)
	}

	if certProvider == CertProviderCertManager && useCertManagerApprover {
		if err = secretsManager.(*certmanageradapter.CertManagerSecretsManager).RegisterCertificateApprover(ctx, mgr); err != nil {
			logrus.WithField("controller", "CertificateRequest").WithError(err).Panic("unable to create controller")
		}
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
	telemetrysender.CredentialsOperatorRunActiveReporter(ctx)
	logrus.Info("starting manager")

	if err := mgr.Start(ctx); err != nil {
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
		logrus.Warningf("failed getting kubesystem UID: %s", err)
		return fmt.Sprintf("rand-%s", uuid.New().String())
	}
	kubeSystemUID = string(kubeSystemNs.UID)

	return kubeSystemUID
}
