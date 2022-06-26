package admission_controller

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"io/ioutil"
	"k8s.io/api/admission/v1"
	"k8s.io/client-go/kubernetes"
	"net/http"
	"time"
)

const (
	DeploymentKind = "Deployment"
)

type AdmissionControllerConfig struct {
	WebhookName    string `env:"WEBHOOK_NAME,notEmpty"`
	ServiceName    string `env:"SERVICE_NAME,notEmpty"`
	Namespace      string `env:"NAMESPACE,notEmpty"`
	Port           int    `env:"PORT,notEmpty" envDefault:"8443"`
	PrivateKeyPath string `env:"PRIVATE_KEY_PATH,notEmpty" envDefault:"${HOME}/key.pem" envExpand:"true"`
	CertPath       string `env:"CERT_PATH,notEmpty" envDefault:"${HOME}/cert.pem" envExpand:"true"`
}

type AdmissionController struct {
	ctx         context.Context
	ctxCancel   context.CancelFunc // This is the cancel func of ctx
	httpsServer *echo.Echo
	config      AdmissionControllerConfig
	kubeClient  *kubernetes.Clientset
}

func NewAdmissionController(ctx context.Context, config AdmissionControllerConfig) (*AdmissionController, error) {
	kubeClient, err := getKubeClient()
	if err != nil {
		return nil, err
	}

	httpsServer := echo.New()
	httpsServer.Use(middleware.Logger())
	cancelCtx, cancelCtxFunc := context.WithCancel(ctx)
	admissionController := AdmissionController{
		ctx:         cancelCtx,
		ctxCancel:   cancelCtxFunc,
		httpsServer: httpsServer,
		kubeClient:  kubeClient,
		config:      config,
	}
	httpsServer.POST("/", admissionController.handleRequest)
	return &admissionController, nil
}

func (ac *AdmissionController) handleRequest(c echo.Context) error {
	body, err := ioutil.ReadAll(c.Request().Body)
	if err != nil {
		return c.String(http.StatusBadRequest, "Could not parse body as json")
	}
	review, err := ac.reviewAndMutate(body)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, review)
}

func (ac *AdmissionController) mutateDeployment(review v1.AdmissionReview) ([]patchValue, error) {
	if review.Request.Kind.Kind != DeploymentKind {
		return nil, fmt.Errorf("expected request kind: %s, got: %s", DeploymentKind, review.Request.Kind.Kind)
	}
	patches := []patchValue{{
		Op:    "add",
		Path:  "/spec/template/metadata/labels/spifferize-id",
		Value: review.Request.Name,
	}}
	// TODO: register to spire-server

	return patches, nil
}

func (ac *AdmissionController) reviewAndMutate(admissionReviewBody []byte) (v1.AdmissionReview, error) {
	logrus.Debugf("Review request triggered for: %s", string(admissionReviewBody))
	var review v1.AdmissionReview
	err := json.Unmarshal(admissionReviewBody, &review)
	if err != nil {
		return v1.AdmissionReview{}, err
	}

	var patches []patchValue
	switch review.Request.Kind.Kind {
	case DeploymentKind:
		patches, err = ac.mutateDeployment(review)
		if err != nil {
			return v1.AdmissionReview{}, err
		}
	default:
		return v1.AdmissionReview{}, fmt.Errorf("got unexpected kind to review: %s", review.Request.Kind.Kind)
	}

	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return v1.AdmissionReview{}, err
	}

	logrus.Infof("Patching service %s on namespace %s", review.Request.Name, review.Request.Namespace)
	logrus.Debugf("Patch: %s", string(patchBytes))
	patchType := v1.PatchTypeJSONPatch
	review.Response = &v1.AdmissionResponse{
		UID:       review.Request.UID,
		Allowed:   true,
		PatchType: &patchType,
		Patch:     patchBytes,
	}
	return review, nil
}

func (ac *AdmissionController) initCert(ctx context.Context) error {
	certBundle, err := generateSelfSignedCertificate(ac.config.ServiceName, ac.config.Namespace)
	if err != nil {
		return err
	}
	err = writeCertToFiles(certBundle, ac.config.CertPath, ac.config.PrivateKeyPath)
	if err != nil {
		return err
	}

	return updateWebHookCA(ctx, ac.kubeClient, ac.config.WebhookName, certBundle.certPem)
}

func (ac *AdmissionController) init(ctx context.Context) error {
	if err := ac.initCert(ctx); err != nil {
		return err
	}

	return nil
}

func (ac *AdmissionController) RunForever() error {
	ctxTimeout, cancel := context.WithTimeout(ac.ctx, 10*time.Second)
	defer cancel()
	err := ac.init(ctxTimeout)
	if err != nil {
		return err
	}

	logrus.Info("WebHook listening on port ", ac.config.Port)
	eGroup := &errgroup.Group{}
	eGroup.Go(func() error {
		defer ac.ctxCancel()
		err := ac.httpsServer.StartTLS(fmt.Sprintf(":%d", ac.config.Port), ac.config.CertPath, ac.config.PrivateKeyPath)
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	// stop gracefully when cancelled by either interrupt or exit of the start method
	<-ac.ctx.Done()
	ctxTimeout, cancel = context.WithTimeout(context.Background(), 10*time.Second) // Background because main context is cancelled at this point
	defer cancel()
	stopErr := ac.httpsServer.Shutdown(ctxTimeout)
	// In case there was error during start, we want to return it (and not the stop error)
	startErr := eGroup.Wait()
	if startErr != nil {
		return startErr
	}
	return stopErr
}
