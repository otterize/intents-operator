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
	"github.com/otterize/intents-operator/src/shared/errors"
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
	"sync"
	"time"
)

const (
	ReviewStatusIndexField = ".status.reviewStatus"
)

// IntentsReconciler reconciles a Intents object
type IntentsReconciler struct {
	client        client.Client
	cloudClient   operator_cloud_client.CloudClient
	approvalState IntentsApprovalState
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

func NewIntentsReconciler(ctx context.Context, client client.Client, cloudClient operator_cloud_client.CloudClient) *IntentsReconciler {
	approvalState := IntentsApprovalState{mutex: &sync.Mutex{}}

	intentsReconciler := &IntentsReconciler{client: client, cloudClient: cloudClient, approvalState: approvalState}
	intentsReconciler.InitIntentsApprovalState(ctx)
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
//     c. Set the up-to-date status to false
//     d. finish reconciliation
//  2. ClientIntents status is not up-to-date and review status is pending:
//     a. The operator should handle intents requests (send to cloud / auto approve)
//     b. finish reconciliation
//  3. ClientIntents is not up-to-date and review status is approved (could be set by the pending intents go routine):
//     a. The operator should create the ApprovedClientIntents
//     b. Set the up-to-date status to true
//     c. finish reconciliation
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

	if intents.Status.UpToDate != false && intents.Status.ObservedGeneration != intents.Generation {
		intentsCopy := intents.DeepCopy()
		intentsCopy.Status.UpToDate = false
		intentsCopy.Status.ObservedGeneration = intents.Generation
		intentsCopy.Status.ReviewStatus = otterizev2alpha1.ReviewStatusPending
		if err := r.client.Status().Patch(ctx, intentsCopy, client.MergeFrom(intents)); err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
		// we have to finish this reconcile loop here so that the group has a fresh copy of the intents
		// and that we don't trigger an infinite loop
		return ctrl.Result{}, nil
	}

	// This is the idempotent way oh handling the intents review status:

	if intents.Status.ReviewStatus != otterizev2alpha1.ReviewStatusApproved {
		// If the intents are not approved, we send them to the right approver and stop the reconcile loop
		// Once the intents will be approved we will get a new reconcile loop
		res, err := r.handleClientIntentsRequests(ctx, *intents)
		return res, errors.Wrap(err)
	}

	// intents.Status.ReviewStatus == otterizev2alpha1.ReviewStatusApproved
	// intents are approved so  we createOrUpdate the approved intents
	err = r.createOrUpdateApprovedIntents(ctx, *intents)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err)
	}

	if intents.DeletionTimestamp == nil && !intents.Status.UpToDate && intents.Status.ReviewStatus == otterizev2alpha1.ReviewStatusApproved {
		intentsCopy := intents.DeepCopy()
		intentsCopy.Status.UpToDate = true
		if err := r.client.Status().Patch(ctx, intentsCopy, client.MergeFrom(intents)); err != nil {
			return ctrl.Result{}, errors.Wrap(err)
		}
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

func (r *IntentsReconciler) InitIntentsApprovalState(ctx context.Context) {
	if r.cloudClient == nil {
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
	err := ctrl.NewControllerManagedBy(mgr).
		For(&otterizev2alpha1.ClientIntents{}).
		WithOptions(controller.Options{RecoverPanic: lo.ToPtr(true)}).
		Complete(r)

	if err != nil {
		return errors.Wrap(err)
	}

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
	return nil
}

func (r *IntentsReconciler) createOrUpdateApprovedIntents(ctx context.Context, intents otterizev2alpha1.ClientIntents) error {
	newApprovedClientIntents := r.buildApprovedClientIntents(intents)
	existingApprovedClientIntents := &otterizev2alpha1.ApprovedClientIntents{}
	err := r.client.Get(ctx, types.NamespacedName{Namespace: newApprovedClientIntents.Namespace, Name: newApprovedClientIntents.Name}, existingApprovedClientIntents)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Wrap(err)
	}
	if k8serrors.IsNotFound(err) {
		return errors.Wrap(r.client.Create(ctx, newApprovedClientIntents))
	}

	if reflect.DeepEqual(existingApprovedClientIntents.Spec, newApprovedClientIntents.Spec) {
		logrus.Debugf("Approved intents %s has not changed", newApprovedClientIntents.Name)
		return nil
	}

	newApprovedClientIntents.ObjectMeta = existingApprovedClientIntents.ObjectMeta
	newApprovedClientIntents.TypeMeta = existingApprovedClientIntents.TypeMeta

	if err := r.client.Patch(ctx, newApprovedClientIntents, client.MergeFrom(existingApprovedClientIntents)); err != nil {
		return errors.Wrap(err)
	}

	return nil
}

func (r *IntentsReconciler) buildApprovedClientIntents(intents otterizev2alpha1.ClientIntents) *otterizev2alpha1.ApprovedClientIntents {
	approvedClientIntents := &otterizev2alpha1.ApprovedClientIntents{}
	approvedClientIntents.FromClientIntents(intents)

	return approvedClientIntents
}

func (r *IntentsReconciler) removeOrphanedApprovedIntents(ctx context.Context) error {
	logrus.Debug("Checking for orphaned approved intents")

	clientIntentsList := &otterizev2alpha1.ClientIntentsList{}
	if err := r.client.List(ctx, clientIntentsList); err != nil {
		return errors.Wrap(err)
	}

	approvedNamesThatShouldExist := goset.FromSlice(lo.Map(
		clientIntentsList.Items, func(intent otterizev2alpha1.ClientIntents, _ int) types.NamespacedName {
			return types.NamespacedName{Namespace: intent.Namespace, Name: intent.ToApprovedIntentsName()}
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
	if intents.Status.ReviewStatus == otterizev2alpha1.ReviewStatusApproved || !intents.DeletionTimestamp.IsZero() {
		// Will be handled elsewhere
		return nil
	}
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
			if err := r.queryAppliedIntentsRequestsStatus(ctx); err != nil {
				logrus.WithError(err).Error("periodic intents approval query failed")
			}
		}
	}
}

func (r *IntentsReconciler) queryAppliedIntentsRequestsStatus(ctx context.Context) error {
	clientIntentsList := &otterizev2alpha1.ClientIntentsList{}
	statusSelector := fields.OneTermEqualSelector(ReviewStatusIndexField, string(otterizev2alpha1.ReviewStatusPending))
	if err := r.client.List(ctx, clientIntentsList, &client.ListOptions{FieldSelector: statusSelector}); err != nil {
		logrus.WithError(err).Error("failed to list intents pending review")
		return errors.Wrap(err)
	}

	if clientIntentsList.Items == nil || len(clientIntentsList.Items) == 0 {
		logrus.Info("No intents pending review")
		return nil
	}

	resourceUIDToIntent := lo.SliceToMap(clientIntentsList.Items, func(intent otterizev2alpha1.ClientIntents) (string, otterizev2alpha1.ClientIntents) {
		return intent.GetRequestID(), intent
	})

	result, err := r.cloudClient.GetAppliedIntentsRequestsStatus(ctx, lo.Keys(resourceUIDToIntent))
	if err != nil {
		logrus.WithError(err).Error("failed to get applied intents requests status")
		return errors.Wrap(err)
	}
	if len(result) == 0 {
		return nil
	}

	return r.handleAppliedIntentsRequestStatusUpdates(ctx, result, resourceUIDToIntent)
}

func (r *IntentsReconciler) handleAppliedIntentsRequestStatusUpdates(ctx context.Context, requestStatus []operator_cloud_client.AppliedIntentsRequestStatus, resourceUIDToIntent map[string]otterizev2alpha1.ClientIntents) error {
	for _, request := range requestStatus {
		if request.Status == graphqlclient.AppliedIntentsRequestStatusLabelPending {
			continue
		}

		clientIntents, ok := resourceUIDToIntent[request.ID]
		if !ok {
			logrus.Errorf("Received status for unknown intents request %s", request.ID)
		}

		// Check the GQL status that returned and request the K8s status accordingly
		reviewStatus := lo.Ternary(request.Status == graphqlclient.AppliedIntentsRequestStatusLabelApproved, otterizev2alpha1.ReviewStatusApproved, otterizev2alpha1.ReviewStatusDenied)
		if err := r.updateClientIntentsStatus(ctx, clientIntents, reviewStatus); err != nil {
			return errors.Wrap(err)
		}
	}

	return nil
}

func (*IntentsReconciler) InitReviewStatusIndex(mgr ctrl.Manager) error {
	return mgr.GetCache().IndexField(
		context.Background(),
		&otterizev2alpha1.ClientIntents{},
		ReviewStatusIndexField,
		func(object client.Object) []string {
			clientIntents := object.(*otterizev2alpha1.ClientIntents)
			return []string{string(clientIntents.Status.ReviewStatus)}
		})
}

func (r *IntentsReconciler) updateClientIntentsStatus(ctx context.Context, intents otterizev2alpha1.ClientIntents, status otterizev2alpha1.ReviewStatus) error {
	intentsCopy := intents.DeepCopy()
	intentsCopy.Status.ReviewStatus = status
	if err := r.client.Status().Patch(ctx, intentsCopy, client.MergeFrom(&intents)); err != nil {
		return errors.Wrap(err)
	}

	return nil
}
