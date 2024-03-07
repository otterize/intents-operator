package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/otterize/credentials-operator/src/controllers/iam"
	"github.com/otterize/credentials-operator/src/controllers/metadata"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"time"
)

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=ignore,groups="",sideEffects=NoneOnDryRun,resources=pods,verbs=create;update,versions=v1,admissionReviewVersions=v1,name=pods.credentials-operator.otterize.com
// +kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=mutatingwebhookconfigurations,verbs=get;update;patch;list

const (
	maxRetries = 5
)

type ServiceAccountAnnotatingPodWebhook struct {
	client  client.Client
	decoder *admission.Decoder
	agents  []iam.IAMCredentialsAgent
}

func NewServiceAccountAnnotatingPodWebhook(mgr manager.Manager, agents []iam.IAMCredentialsAgent) *ServiceAccountAnnotatingPodWebhook {
	return &ServiceAccountAnnotatingPodWebhook{
		client:  mgr.GetClient(),
		decoder: admission.NewDecoder(mgr.GetScheme()),
		agents:  agents,
	}
}

func (a *ServiceAccountAnnotatingPodWebhook) handleOnce(ctx context.Context, pod corev1.Pod, dryRun bool) (outputPod corev1.Pod, patched bool, successMsg string, err error) {
	logger := logrus.WithField("name", pod.Name).WithField("namespace", pod.Namespace)
	if pod.DeletionTimestamp != nil {
		return pod, false, "no webhook handling if pod is terminating", nil
	}

	if pod.Labels == nil {
		return pod, false, "no create IAM role label - no modifications made", nil
	}

	var serviceAccount corev1.ServiceAccount
	err = a.client.Get(ctx, types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      pod.Spec.ServiceAccountName,
	}, &serviceAccount)
	if err != nil {
		return corev1.Pod{}, false, "", fmt.Errorf("could not get service account: %w", err)
	}

	updatedServiceAccount := serviceAccount.DeepCopy()

	if updatedServiceAccount.Annotations == nil {
		updatedServiceAccount.Annotations = make(map[string]string)
	}

	if updatedServiceAccount.Labels == nil {
		updatedServiceAccount.Labels = make(map[string]string)
	}

	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}

	hasUpdates := false
	for _, agent := range a.agents {
		updated, err := agent.OnPodAdmission(ctx, &pod, updatedServiceAccount)
		if err != nil {
			return corev1.Pod{}, false, "", fmt.Errorf("failed to handle pod admission: %w", err)
		}
		hasUpdates = hasUpdates || updated
	}

	if !hasUpdates {
		logger.Debugf("pod doesn't have any create IAM role label, skipping")
		return pod, false, "no create IAM role label - no modifications made", nil
	}

	updatedServiceAccount.Labels[metadata.OtterizeServiceAccountLabel] = metadata.OtterizeServiceAccountHasPodsValue

	if !dryRun {
		err = a.client.Patch(ctx, updatedServiceAccount, client.MergeFrom(&serviceAccount))
		if err != nil {
			return corev1.Pod{}, false, "", fmt.Errorf("could not patch service account: %w", err)
		}
	}

	controllerutil.AddFinalizer(&pod, metadata.IAMRoleFinalizer)
	return pod, true, "pod and service account updated to create IAM role", nil
}

// dryRun: should not cause any modifications except to the Pod in the request.
func (a *ServiceAccountAnnotatingPodWebhook) handleWithRetriesOnConflictOrNotFound(ctx context.Context, pod corev1.Pod, dryRun bool) (outputPod corev1.Pod, patched bool, successMsg string, err error) {
	logger := logrus.WithField("name", pod.Name).WithField("namespace", pod.Namespace)
	for attempt := 0; attempt < maxRetries; attempt++ {
		logger.Debugf("Handling pod (attempt %d out of %d)", attempt+1, maxRetries)
		outputPod, patched, successMsg, err = a.handleOnce(ctx, *pod.DeepCopy(), dryRun)
		if err != nil {
			if k8serrors.IsConflict(err) || k8serrors.IsNotFound(err) {
				logger.WithError(err).Errorf("failed to handle pod due to conflict, retrying in 1 second (attempt %d out of %d)", attempt+1, 3)
				time.Sleep(1 * time.Second)
				continue
			}
			return corev1.Pod{}, false, "", errors.Wrap(err)
		}
		return outputPod, patched, successMsg, nil
	}
	if err != nil {
		return corev1.Pod{}, false, "", errors.Wrap(err)
	}
	panic("unreachable - must have received error or it would have exited in the for loop")
}

func (a *ServiceAccountAnnotatingPodWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := corev1.Pod{}
	err := a.decoder.Decode(req, &pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	logger := logrus.WithField("name", pod.Name).WithField("namespace", pod.Namespace)
	logger.Debug("Got webhook call for pod")

	pod, patched, successMsg, err := a.handleWithRetriesOnConflictOrNotFound(ctx, pod, req.DryRun != nil && *req.DryRun)
	if err != nil {
		logger.WithError(err).Errorf("failed to annotate service account, but pod admitted to ensure success")
		return admission.Allowed("pod admitted, but failed to annotate service account, see warnings").WithWarnings(err.Error())
	}

	if !patched {
		return admission.Allowed(successMsg)
	}

	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}
