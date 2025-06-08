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

package controllers

import (
	"context"
	"github.com/amit7itz/goset"
	otterizev2alpha1 "github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/operator/mirrorevents"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/operator_cloud_client"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/graphqlclient"
	"github.com/otterize/intents-operator/src/shared/otterizecloud/otterizecloudclient"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sync"
	"time"
)

const (
	ReviewStatusIndexField              = ".status.reviewStatus"
	ReasonApprovedIntentsCreated        = "ApprovedIntentsCreated"
	ReasonReviewStatusChanged           = "ReviewStatusChanged"
	ReasonApprovedIntentsUpdated        = "ApprovedIntentsUpdated"
	ReasonApprovedIntentsCreationFailed = "ApprovedIntentsCreationFailed"
)

// IntentsReconciler reconciles a Intents object
type IntentsReconciler struct {
	client        client.Client
	cloudClient   operator_cloud_client.CloudClient
	approvalState IntentsApprovalState
	injectablerecorder.InjectableRecorder
}

type IntentsApprovalState struct {
	mutex          *sync.Mutex
	approvalMethod IntentsApprover
}

type IntentsApprover string

const (
	ApprovalMethodCloudApproval IntentsApprover = "CloudApproval"
	ApprovalMethodAutoApproval  IntentsApprover = "AutoApproval" // auto approve, for operators not integrated with Otterize cloud
)

func NewIntentsReconciler(ctx context.Context, client client.Client, cloudClient operator_cloud_client.CloudClient, enableCloudApproval bool) *IntentsReconciler {
	approvalState := IntentsApprovalState{mutex: &sync.Mutex{}}

	intentsReconciler := &IntentsReconciler{client: client, cloudClient: cloudClient, approvalState: approvalState}
	intentsReconciler.initIntentsApprovalState(ctx, enableCloudApproval)
	return intentsReconciler
}

//+kubebuilder:rbac:groups=k8s.otterize.com,resources=approvedclientintents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=postgresqlserverconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=mysqlserverconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=clientintents/finalizers,verbs=update
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=approvedclientintents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.otterize.com,resources=approvedclientintents/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;update;patch;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;update;patch;list;watch;create
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get
//+kubebuilder:rbac:groups="networking.k8s.io",resources=networkpolicies,verbs=get;update;patch;list;watch;delete;create
//+kubebuilder:rbac:groups="admissionregistration.k8s.io",resources=validatingwebhookconfigurations,verbs=get;update;patch;list
//+kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=get;list;watch;update;create;patch
//+kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=get;list;watch;update;create;patch
// +kubebuilder:rbac:groups=iam.cnrm.cloud.google.com,resources=iampartialpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile - main reconciliation loop
// The ClientIntents lifecycle is as follows:
//  1. ClientIntents is created/updated, so it's generation is increased and the review status seems approved/empty. The operator should:
//     a. Set the review status to pending
//     b. Set the observed generation to the new generation
//     c. set upToData to false
//     d. finish reconciliation
//  2. ClientIntents review status is pending:
//     a. The operator should handle intents requests (send to cloud / auto approve)
//     b. finish reconciliation
//  3. ClientIntents review status is approved (could be set by the pending-intents go routine):
//     a. The operator should create/update/DoNothing the ApprovedClientIntents
//     c. finish reconciliation
//
// Status.UpToDate should reflect if the intents have been applied successfully to the cluster,
// therefore it is being set to true only after the *approvedClientIntents* is reconciled successfully
func (r *IntentsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if err := r.removeOrphanedApprovedIntents(ctx); err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	intents := &otterizev2alpha1.ClientIntents{}
	err := r.client.Get(ctx, req.NamespacedName, intents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err)
	}

	// delete approved intents if client intents is being deleted
	if !intents.DeletionTimestamp.IsZero() {
		return r.handleClientIntentDeletion(ctx, intents)
	}

	// add finalizer if not exists
	if !controllerutil.ContainsFinalizer(intents, otterizev2alpha1.ClientIntentsFinalizerName) {
		return r.addFinalizer(ctx, intents)
	}

	// If the observed generation is not the same as the generation, we update the observed generation and the review status
	if intents.Status.ObservedGeneration != intents.Generation {
		return r.handleClientIntentsChanged(ctx, intents)
	}

	if intents.Status.ReviewStatus != otterizev2alpha1.ReviewStatusApproved {
		// If the intents are not approved, we send them to the right approver and stop the reconcile loop
		// Once the intents will be approved we will get a new reconcile loop
		return r.handleClientIntentsRequests(ctx, *intents)
	}

	// intents.Status.ReviewStatus == otterizev2alpha1.ReviewStatusApproved
	return r.handleApprovedClientIntents(ctx, intents)
}

func (r *IntentsReconciler) handleApprovedClientIntents(ctx context.Context, intents *otterizev2alpha1.ClientIntents) (ctrl.Result, error) {
	if err := r.createOrUpdateApprovedIntents(ctx, *intents); err != nil {
		r.RecordWarningEventf(intents, ReasonApprovedIntentsCreationFailed, "Failed to create approved intents: %s", err)
		return ctrl.Result{}, errors.Wrap(err)
	}
	return ctrl.Result{}, nil
}

func (r *IntentsReconciler) handleClientIntentsChanged(ctx context.Context, intents *otterizev2alpha1.ClientIntents) (ctrl.Result, error) {
	intentsCopy := intents.DeepCopy()
	intentsCopy.Status.ObservedGeneration = intents.Generation
	intentsCopy.Status.ReviewStatus = otterizev2alpha1.ReviewStatusPending
	intentsCopy.Status.UpToDate = false
	if err := r.client.Status().Patch(ctx, intentsCopy, client.MergeFrom(intents)); err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}
	r.RecordNormalEventf(intents, ReasonReviewStatusChanged, "Updated review status to %s (for generation %d)", otterizev2alpha1.ReviewStatusPending, intents.Generation)
	// we have to finish this reconcile loop here so that the group has a fresh copy of the intents
	// and that we don't trigger an infinite loop
	return ctrl.Result{}, nil
}

func (r *IntentsReconciler) addFinalizer(ctx context.Context, intents *otterizev2alpha1.ClientIntents) (ctrl.Result, error) {
	intentsCopy := intents.DeepCopy()
	controllerutil.AddFinalizer(intentsCopy, otterizev2alpha1.ClientIntentsFinalizerName)
	if err := r.client.Update(ctx, intentsCopy); err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}
	return ctrl.Result{}, nil
}

func (r *IntentsReconciler) handleClientIntentDeletion(ctx context.Context, intents *otterizev2alpha1.ClientIntents) (ctrl.Result, error) {
	approvedIntents := &otterizev2alpha1.ApprovedClientIntents{}
	err := r.client.Get(ctx, types.NamespacedName{Namespace: intents.Namespace, Name: intents.Name}, approvedIntents)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// if approvedClientIntents not found, we can remove the finalizer and finish the reconcile loop
			return r.removeFinalizer(ctx, intents)
		}
		return ctrl.Result{}, errors.Wrap(err)
	}
	if err := r.client.Delete(ctx, approvedIntents); err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}
	return ctrl.Result{}, nil
}

func (r *IntentsReconciler) removeFinalizer(ctx context.Context, intents *otterizev2alpha1.ClientIntents) (ctrl.Result, error) {
	intentsCopy := intents.DeepCopy()
	if !controllerutil.ContainsFinalizer(intentsCopy, otterizev2alpha1.ClientIntentsFinalizerName) {
		return ctrl.Result{}, nil
	}
	controllerutil.RemoveFinalizer(intentsCopy, otterizev2alpha1.ClientIntentsFinalizerName)
	if err := r.client.Update(ctx, intentsCopy); err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}
	return ctrl.Result{}, nil
}

func (r *IntentsReconciler) handleClientIntentsRequests(ctx context.Context, intents otterizev2alpha1.ClientIntents) (ctrl.Result, error) {
	if !intents.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	approvalMethod := r.approvalState.approvalMethod
	switch approvalMethod {
	case ApprovalMethodCloudApproval:
		if err := r.handleCloudAppliedIntentsRequests(ctx, intents); err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
	case ApprovalMethodAutoApproval:
		if err := r.handleAutoApprovalForIntents(ctx, intents); err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *IntentsReconciler) initIntentsApprovalState(ctx context.Context, enableCloudApproval bool) {
	if r.cloudClient == nil || !enableCloudApproval {
		r.approvalState.approvalMethod = ApprovalMethodAutoApproval
		return
	}

	r.approvalState.mutex.Lock()
	defer r.approvalState.mutex.Unlock()

	r.approvalState.approvalMethod = ApprovalMethodCloudApproval
	go r.periodicQueryAppliedIntentsRequestsStatus(ctx)
}

// SetupWithManager sets up the controller with the Manager.
func (r *IntentsReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := r.initReviewStatusIndex(mgr)
	if err != nil {
		return errors.Wrap(err)
	}
	err = ctrl.NewControllerManagedBy(mgr).
		For(&otterizev2alpha1.ClientIntents{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)

	if err != nil {
		return errors.Wrap(err)
	}

	r.InjectRecorder(mirrorevents.GetMirrorToClientIntentsEventRecorderFor(mgr, "intents-operator"))

	return nil
}

func (r *IntentsReconciler) handleAutoApprovalForIntents(ctx context.Context, intents otterizev2alpha1.ClientIntents) error {
	if intents.Status.ReviewStatus == otterizev2alpha1.ReviewStatusApproved {
		return nil
	}
	intentsCopy := intents.DeepCopy()
	intentsCopy.Status.ReviewStatus = otterizev2alpha1.ReviewStatusApproved

	if err := r.client.Status().Patch(ctx, intentsCopy, client.MergeFrom(&intents)); err != nil {
		return errors.Wrap(err)
	}
	r.RecordNormalEventf(&intents, ReasonReviewStatusChanged, "Generation %d: Updated review status to %s", intents.Generation, otterizev2alpha1.ReviewStatusApproved)
	return nil
}

func (r *IntentsReconciler) createOrUpdateApprovedIntents(ctx context.Context, intents otterizev2alpha1.ClientIntents) error {
	newApprovedClientIntents := r.buildApprovedClientIntents(intents)
	// set owner reference
	err := controllerutil.SetOwnerReference(&intents, newApprovedClientIntents, r.client.Scheme())
	if err != nil {
		return errors.Wrap(err)
	}

	existingApprovedClientIntents := &otterizev2alpha1.ApprovedClientIntents{}
	err = r.client.Get(ctx, types.NamespacedName{Namespace: newApprovedClientIntents.Namespace, Name: newApprovedClientIntents.Name}, existingApprovedClientIntents)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err)
	}
	if k8serrors.IsNotFound(err) {
		if err := r.client.Create(ctx, newApprovedClientIntents); err != nil {

			return errors.Wrap(err)
		}
		r.RecordNormalEventf(&intents, ReasonApprovedIntentsCreated, "Created approved intents %s", newApprovedClientIntents.Name)
		return nil
	}

	if !r.hasApprovedClientIntentsChanged(existingApprovedClientIntents, newApprovedClientIntents) {
		logrus.Debugf("Approved intents %s has not changed", newApprovedClientIntents.Name)
		return nil
	}

	patchedClientIntents := existingApprovedClientIntents.DeepCopy()
	patchedClientIntents.Spec = newApprovedClientIntents.Spec
	patchedClientIntents.OwnerReferences = newApprovedClientIntents.OwnerReferences

	if err := r.client.Patch(ctx, patchedClientIntents, client.MergeFrom(existingApprovedClientIntents)); err != nil {
		return errors.Wrap(err)
	}
	r.RecordNormalEventf(&intents, ReasonApprovedIntentsUpdated, "Updated approved intents %s", newApprovedClientIntents.Name)

	return nil
}

func (r *IntentsReconciler) buildApprovedClientIntents(intents otterizev2alpha1.ClientIntents) *otterizev2alpha1.ApprovedClientIntents {
	approvedClientIntents := &otterizev2alpha1.ApprovedClientIntents{}
	approvedClientIntents.FromClientIntents(intents)

	return approvedClientIntents
}

func (r *IntentsReconciler) hasApprovedClientIntentsChanged(existingApprovedIntents, newApprovedIntents *otterizev2alpha1.ApprovedClientIntents) bool {
	if !reflect.DeepEqual(existingApprovedIntents.Spec, newApprovedIntents.Spec) {
		return true
	}
	oldOwnerReferences := existingApprovedIntents.GetOwnerReferences()
	newOwnerReferences := newApprovedIntents.GetOwnerReferences()
	if len(oldOwnerReferences) != 1 || len(newOwnerReferences) != 1 {
		return true
	}

	if oldOwnerReferences[0].UID != newOwnerReferences[0].UID {
		return true
	}

	return false
}

func (r *IntentsReconciler) removeOrphanedApprovedIntents(ctx context.Context) error {
	logrus.Debug("Checking for orphaned approved intents")

	clientIntentsList := &otterizev2alpha1.ClientIntentsList{}
	if err := r.client.List(ctx, clientIntentsList); err != nil {
		return errors.Wrap(err)
	}

	approvedNamesThatShouldExist := goset.FromSlice(lo.Map(
		clientIntentsList.Items, func(intent otterizev2alpha1.ClientIntents, _ int) types.NamespacedName {
			return types.NamespacedName{Namespace: intent.Namespace, Name: intent.Name}
		}))

	approvedClientIntentsList := &otterizev2alpha1.ApprovedClientIntentsList{}
	if err := r.client.List(ctx, approvedClientIntentsList); err != nil {
		return errors.Wrap(err)
	}

	for _, approvedClientIntents := range approvedClientIntentsList.Items {
		if approvedNamesThatShouldExist.Contains(types.NamespacedName{Namespace: approvedClientIntents.Namespace, Name: approvedClientIntents.Name}) {
			continue
		}
		logrus.Infof("Deleting orphaned approved intents %s", approvedClientIntents.Name)
		if err := r.client.Delete(ctx, &approvedClientIntents); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (r *IntentsReconciler) handleCloudAppliedIntentsRequests(ctx context.Context, intents otterizev2alpha1.ClientIntents) error {
	if intents.Status.ReviewStatus == "" {
		if err := r.updateClientIntentsStatus(ctx, intents, otterizev2alpha1.ReviewStatusPending); err != nil {
			return errors.Wrap(err)
		}
	}

	// Convert to list to save the hellish conversion to cloud format
	intentsList := &otterizev2alpha1.ClientIntentsList{
		Items: []otterizev2alpha1.ClientIntents{intents},
	}
	cloudIntents, err := intentsList.FormatAsOtterizeIntentsRequests(ctx, r.client)
	if err != nil {
		return errors.Wrap(err)
	}

	return errors.Wrap(r.cloudClient.ReportAppliedIntentsRequest(ctx, cloudIntents))
}

func (r *IntentsReconciler) periodicQueryAppliedIntentsRequestsStatus(ctx context.Context) {
	approvalQueryTicker := time.NewTicker(time.Second * time.Duration(viper.GetInt(otterizecloudclient.IntentsApprovalQueryIntervalKey)))

	for {
		select {
		case <-ctx.Done():
			return
		case <-approvalQueryTicker.C:
			if err := r.findAndHandlePendingRequests(ctx); err != nil {
				logrus.WithError(err).Error("periodic intents approval query failed")
			}
		}
	}
}

func (r *IntentsReconciler) findAndHandlePendingRequests(ctx context.Context) error {
	clientIntentsList := &otterizev2alpha1.ClientIntentsList{}
	statusSelector := fields.OneTermEqualSelector(ReviewStatusIndexField, string(otterizev2alpha1.ReviewStatusPending))
	if err := r.client.List(ctx, clientIntentsList, &client.ListOptions{FieldSelector: statusSelector}); err != nil {
		logrus.WithError(err).Error("failed to list intents pending review")
		return errors.Wrap(err)
	}

	if len(clientIntentsList.Items) == 0 {
		logrus.Debug("No intents pending review")
		// Still reporting to the cloud since the cloud might have to mark the requests as stale
	}

	return errors.Wrap(r.handlePendingRequests(ctx, clientIntentsList))
}

func (r *IntentsReconciler) handlePendingRequests(ctx context.Context, pendingClientIntentsList *otterizev2alpha1.ClientIntentsList) error {
	resourceUIDToIntent := lo.SliceToMap(pendingClientIntentsList.Items, func(intent otterizev2alpha1.ClientIntents) (graphqlclient.IntentRequestResourceGeneration, otterizev2alpha1.ClientIntents) {
		return intent.GetResourceGeneration(), intent
	})

	requestStatuses, err := r.cloudClient.GetAppliedIntentsRequestsStatus(ctx, lo.Keys(resourceUIDToIntent))
	if err != nil {
		logrus.WithError(err).Error("failed to get applied intents requests status")
		return errors.Wrap(err)
	}
	if len(requestStatuses) == 0 {
		return nil
	}

	for _, request := range requestStatuses {
		clientIntents, ok := resourceUIDToIntent[request.ResourceGeneration]
		if !ok {
			// Should not happen
			logrus.Errorf("Received status for unknown intents request %v", request.ResourceGeneration)
			continue
		}

		if err := r.handleRequestStatusResponse(ctx, request, clientIntents); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (r *IntentsReconciler) handleRequestStatusResponse(ctx context.Context, request operator_cloud_client.AppliedIntentsRequestStatus, clientIntents otterizev2alpha1.ClientIntents) error {
	if request.Status == graphqlclient.AppliedIntentsRequestStatusLabelPending {
		logrus.Debugf("Received pending status for intents request %v", request.ResourceGeneration)
		return nil
	}

	// Check the GQL status that returned and request the K8s status accordingly
	reviewStatus := lo.Ternary(request.Status == graphqlclient.AppliedIntentsRequestStatusLabelApproved, otterizev2alpha1.ReviewStatusApproved, otterizev2alpha1.ReviewStatusDenied)
	if err := r.updateClientIntentsStatus(ctx, clientIntents, reviewStatus); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (r *IntentsReconciler) updateClientIntentsStatus(ctx context.Context, intents otterizev2alpha1.ClientIntents, status otterizev2alpha1.ReviewStatus) error {
	intentsCopy := intents.DeepCopy()
	intentsCopy.Status.ReviewStatus = status
	if err := r.client.Status().Patch(ctx, intentsCopy, client.MergeFrom(&intents)); err != nil {
		return errors.Wrap(err)
	}
	r.RecordNormalEventf(&intents, ReasonReviewStatusChanged, "Updated review status to %s (for generation %d)", status, intents.Generation)
	logrus.WithFields(logrus.Fields{
		"clientIntents.name":       intentsCopy.Name,
		"clientIntents.namespaces": intentsCopy.Namespace,
		"status":                   status,
	}).Info("Updated client intents status")

	return nil
}

func (*IntentsReconciler) initReviewStatusIndex(mgr ctrl.Manager) error {
	return mgr.GetCache().IndexField(
		context.Background(),
		&otterizev2alpha1.ClientIntents{},
		ReviewStatusIndexField,
		func(object client.Object) []string {
			clientIntents := object.(*otterizev2alpha1.ClientIntents)
			return []string{string(clientIntents.Status.ReviewStatus)}
		})
}
