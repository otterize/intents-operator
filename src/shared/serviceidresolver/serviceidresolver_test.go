package serviceidresolver

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/errors"
	serviceidresolvermocks "github.com/otterize/intents-operator/src/shared/serviceidresolver/mocks"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/podownerresolver"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

type MatchingLabelsSelectorMatcher struct {
	expected client.MatchingLabelsSelector
}

func (m *MatchingLabelsSelectorMatcher) Matches(x interface{}) bool {
	if x == nil {
		return false
	}
	matchingLabels, ok := x.(client.MatchingLabelsSelector)
	if !ok {
		return false
	}
	return m.expected.String() == matchingLabels.String()
}

func (m *MatchingLabelsSelectorMatcher) String() string {
	return m.expected.String()
}

type ServiceIdResolverTestSuite struct {
	suite.Suite
	Client     *serviceidresolvermocks.MockClient
	RESTMapper *serviceidresolvermocks.MockRESTMapper
	Resolver   *Resolver
}

func (s *ServiceIdResolverTestSuite) SetupTest() {
	controller := gomock.NewController(s.T())
	s.Client = serviceidresolvermocks.NewMockClient(controller)
	s.RESTMapper = serviceidresolvermocks.NewMockRESTMapper(controller)
	s.Client.EXPECT().RESTMapper().Return(s.RESTMapper).AnyTimes()
	s.Resolver = NewResolver(s.Client)
}

func (s *ServiceIdResolverTestSuite) TestResolveClientIntentToPod_PodExists() {
	serviceName := "coolservice"
	namespace := "coolnamespace"
	SAName := "backendservice"

	intent := v2alpha1.ApprovedClientIntents{Spec: &v2alpha1.IntentsSpec{Workload: v2alpha1.Workload{Name: serviceName}}, ObjectMeta: metav1.ObjectMeta{Namespace: namespace}}
	ls, _, err := v2alpha1.ServiceIdentityToLabelsForWorkloadSelection(context.Background(), s.Client, intent.ToServiceIdentity())
	s.Require().NoError(err)

	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace}, Spec: corev1.PodSpec{ServiceAccountName: SAName}}

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		gomock.Eq(&client.ListOptions{Namespace: namespace}),
		gomock.Eq(ls),
	).Do(func(_ any, podList *corev1.PodList, _ ...any) {
		podList.Items = append(podList.Items, pod)
	})

	resolvedPod, err := s.Resolver.ResolveClientIntentToPod(context.Background(), intent)
	resultSAName := resolvedPod.Spec.ServiceAccountName
	s.Require().NoError(err)
	s.Require().Equal(SAName, resultSAName)
}

func (s *ServiceIdResolverTestSuite) TestResolveClientIntentToPod_PodDoesntExist() {
	serviceName := "coolservice"
	namespace := "coolnamespace"

	intent := v2alpha1.ApprovedClientIntents{Spec: &v2alpha1.IntentsSpec{Workload: v2alpha1.Workload{Name: serviceName}}, ObjectMeta: metav1.ObjectMeta{Namespace: namespace}}
	ls, _, err := v2alpha1.ServiceIdentityToLabelsForWorkloadSelection(context.Background(), s.Client, intent.ToServiceIdentity())
	s.Require().NoError(err)

	s.Client.EXPECT().List(
		gomock.Any(),
		gomock.AssignableToTypeOf(&corev1.PodList{}),
		gomock.Eq(&client.ListOptions{Namespace: namespace}),
		gomock.Eq(ls),
	).Do(func(_ any, podList *corev1.PodList, _ ...any) {})

	pod, err := s.Resolver.ResolveClientIntentToPod(context.Background(), intent)
	s.Require().True(errors.Is(err, ErrPodNotFound))
	s.Require().Equal(corev1.Pod{}, pod)
}

func (s *ServiceIdResolverTestSuite) TestGetPodAnnotatedName_PodExists() {
	podName := "coolpod"
	podNamespace := "coolnamespace"
	serviceName := "coolservice"

	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: podNamespace, Annotations: map[string]string{viper.GetString(podownerresolver.WorkloadNameOverrideAnnotationKey): serviceName}}}
	id, err := s.Resolver.ResolvePodToServiceIdentity(context.Background(), &pod)
	s.Require().NoError(err)
	s.Require().Equal(serviceName, id.Name)
}

func (s *ServiceIdResolverTestSuite) TestGetPodAnnotatedName_PodMissingAnnotation() {
	podName := "coolpod"
	podNamespace := "coolnamespace"

	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: podNamespace}}

	id, err := s.Resolver.ResolvePodToServiceIdentity(context.Background(), &pod)
	s.Require().Nil(err)
	s.Require().Equal(podName, id.Name)
}

func (s *ServiceIdResolverTestSuite) TestDeploymentNameWithDotsReplacedByUnderscore() {
	deploymentName := "cool-versioned-application.4.2.0"
	podName := "cool-pod-1234567890-12345"
	serviceName := "cool-versioned-application_4_2_0"
	podNamespace := "cool-namespace"

	// Create a pod with reference to the deployment with dots in the name
	myPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Deployment",
					Name:       deploymentName,
					APIVersion: "apps/v1",
				},
			},
		},
	}

	deploymentAsObject := unstructured.Unstructured{}
	deploymentAsObject.SetName(deploymentName)
	deploymentAsObject.SetNamespace(podNamespace)
	deploymentAsObject.SetKind("Deployment")
	deploymentAsObject.SetAPIVersion("apps/v1")

	emptyObject := &unstructured.Unstructured{}
	emptyObject.SetKind("Deployment")
	emptyObject.SetAPIVersion("apps/v1")
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: deploymentName, Namespace: podNamespace}, emptyObject).Do(
		func(_ context.Context, _ types.NamespacedName, obj *unstructured.Unstructured, _ ...any) error {
			deploymentAsObject.DeepCopyInto(obj)
			return nil
		})

	service, err := s.Resolver.ResolvePodToServiceIdentity(context.Background(), &myPod)
	s.Require().NoError(err)
	s.Require().Equal(serviceName, service.Name)
}

func (s *ServiceIdResolverTestSuite) TestDeploymentReadForbidden() {
	deploymentName := "best-deployment-ever"
	podName := "cool-pod-1234567890-12345"
	podNamespace := "cool-namespace"

	// Create a pod with reference to the deployment with dots in the name
	myPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Deployment",
					Name:       deploymentName,
					APIVersion: "apps/v1",
				},
			},
		},
	}

	emptyObject := &unstructured.Unstructured{}
	emptyObject.SetKind("Deployment")
	emptyObject.SetAPIVersion("apps/v1")

	forbiddenError := apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "Deployment"}, deploymentName, errors.New("forbidden"))
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: deploymentName, Namespace: podNamespace}, emptyObject).Return(forbiddenError)

	service, err := s.Resolver.ResolvePodToServiceIdentity(context.Background(), &myPod)
	s.Require().NoError(err)
	s.Require().Equal(deploymentName, service.Name)
}

func (s *ServiceIdResolverTestSuite) TestDeploymentRead() {
	deploymentName := "best-deployment-ever"
	podName := "cool-pod-1234567890-12345"
	podNamespace := "cool-namespace"

	// Create a pod with reference to the deployment with dots in the name
	myPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Deployment",
					Name:       deploymentName,
					APIVersion: "apps/v1",
				},
			},
		},
	}

	deploymentAsObject := unstructured.Unstructured{}
	deploymentAsObject.SetName(deploymentName)
	deploymentAsObject.SetNamespace(podNamespace)
	deploymentAsObject.SetKind("Deployment")
	deploymentAsObject.SetAPIVersion("apps/v1")

	emptyObject := &unstructured.Unstructured{}
	emptyObject.SetKind("Deployment")
	emptyObject.SetAPIVersion("apps/v1")
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: deploymentName, Namespace: podNamespace}, emptyObject).Do(
		func(_ context.Context, _ types.NamespacedName, obj *unstructured.Unstructured, _ ...any) error {
			deploymentAsObject.DeepCopyInto(obj)
			return nil
		})

	service, err := s.Resolver.ResolvePodToServiceIdentity(context.Background(), &myPod)
	s.Require().NoError(err)
	s.Require().Equal(deploymentName, service.Name)
}

func (s *ServiceIdResolverTestSuite) TestJobWithNoParent() {
	jobName := "my-crappy-1001-job-name-1234567890-12345"
	podName := "cool-pod-1234567890-12345"
	podNamespace := "cool-namespace"
	imageName := "cool-image"

	// Create a pod with reference to the deployment with dots in the name
	myPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Job",
					Name:       jobName,
					APIVersion: "batch/v1",
				},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "cool-container",
					Image: fmt.Sprintf("353146681200.dkr.ecr.us-west-2.amazonaws.com/%s:some-tag", imageName),
				},
			},
		},
	}

	deploymentAsObject := unstructured.Unstructured{}
	deploymentAsObject.SetName(jobName)
	deploymentAsObject.SetNamespace(podNamespace)
	deploymentAsObject.SetKind("Job")
	deploymentAsObject.SetAPIVersion("batch/v1")

	emptyObject := &unstructured.Unstructured{}
	emptyObject.SetKind("Job")
	emptyObject.SetAPIVersion("batch/v1")
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: jobName, Namespace: podNamespace}, emptyObject).Do(
		func(_ context.Context, _ types.NamespacedName, obj *unstructured.Unstructured, _ ...any) error {
			deploymentAsObject.DeepCopyInto(obj)
			return nil
		})

	viper.Set(podownerresolver.UseImageNameForServiceIDForJobs, false)
	service, err := s.Resolver.ResolvePodToServiceIdentity(context.Background(), &myPod)
	s.Require().NoError(err)
	s.Require().Equal(jobName, service.Name)

	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: jobName, Namespace: podNamespace}, emptyObject).Do(
		func(_ context.Context, _ types.NamespacedName, obj *unstructured.Unstructured, _ ...any) error {
			deploymentAsObject.DeepCopyInto(obj)
			return nil
		})

	viper.Set(podownerresolver.UseImageNameForServiceIDForJobs, true)
	service, err = s.Resolver.ResolvePodToServiceIdentity(context.Background(), &myPod)
	s.Require().NoError(err)
	s.Require().Equal(imageName, service.Name)
}
func (s *ServiceIdResolverTestSuite) TestJobCronJobWithRemovedVersion() {
	jobName := "my-crappy-1001-job-name-1234567890-12345"
	podName := "cool-pod-1234567890-12345"
	podNamespace := "cool-namespace"
	cronJobName := "cool-cron-job"

	// Create a pod with reference to the deployment with dots in the name
	myPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Job",
					Name:       jobName,
					APIVersion: "batch/v1beta1",
				},
			},
		},
	}

	jobAsObject := unstructured.Unstructured{}
	jobAsObject.SetName(jobName)
	jobAsObject.SetNamespace(podNamespace)
	jobAsObject.SetKind("Job")
	jobAsObject.SetAPIVersion("batch/v1beta1")
	jobAsObject.SetOwnerReferences(
		[]metav1.OwnerReference{
			{
				Kind:       "CronJob",
				Name:       cronJobName,
				APIVersion: "batch/v1beta1",
			},
		})

	jobEmptyObject := &unstructured.Unstructured{}
	jobEmptyObject.SetKind("Job")
	jobEmptyObject.SetAPIVersion("batch/v1beta1")
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: jobName, Namespace: podNamespace}, jobEmptyObject).Do(
		func(_ context.Context, _ types.NamespacedName, obj *unstructured.Unstructured, _ ...any) error {
			jobAsObject.DeepCopyInto(obj)
			return nil
		})
	s.RESTMapper.EXPECT().RESTMapping(schema.GroupKind{
		Group: "batch",
		Kind:  "CronJob",
	}, "v1beta1").Return(nil, &meta.NoKindMatchError{})

	cronJobRESTMapping := &meta.RESTMapping{
		Resource: schema.GroupVersionResource{
			Group:    "batch",
			Version:  "v1",
			Resource: "cronjobs",
		},
		GroupVersionKind: schema.GroupVersionKind{
			Group:   "batch",
			Version: "v1",
			Kind:    "CronJob",
		},
		Scope: meta.RESTScopeRoot,
	}

	s.RESTMapper.EXPECT().RESTMapping(schema.GroupKind{
		Group: "batch",
		Kind:  "CronJob",
	}, "").Return(cronJobRESTMapping, nil)

	cronJobEmptyObject := &unstructured.Unstructured{}
	cronJobEmptyObject.SetKind("CronJob")
	cronJobEmptyObject.SetAPIVersion("batch/v1")
	cronjobAsObject := unstructured.Unstructured{}
	cronjobAsObject.SetName(cronJobName)
	cronjobAsObject.SetNamespace(podNamespace)
	cronjobAsObject.SetKind("CronJob")
	cronjobAsObject.SetAPIVersion("batch/v1")
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: cronJobName, Namespace: podNamespace}, cronJobEmptyObject).Do(
		func(_ context.Context, _ types.NamespacedName, obj *unstructured.Unstructured, _ ...any) error {
			cronjobAsObject.DeepCopyInto(obj)
			return nil
		})

	service, err := s.Resolver.ResolvePodToServiceIdentity(context.Background(), &myPod)
	s.Require().NoError(err)
	s.Require().Equal(cronJobName, service.Name)

}

func (s *ServiceIdResolverTestSuite) TestPodOwnerWithBadKind() {
	ownerName := "my-owner-name-1234567890-12345"
	podName := "cool-pod-1234567890-12345"
	podNamespace := "cool-namespace"

	// Create a pod with reference to an owner with a bad kind
	myPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "BadKind",
					Name:       ownerName,
					APIVersion: "wrong.api/v1beta1",
				},
			},
		},
	}

	ownerEmptyObject := &unstructured.Unstructured{}
	ownerEmptyObject.SetKind("BadKind")
	ownerEmptyObject.SetAPIVersion("wrong.api/v1beta1")
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: ownerName, Namespace: podNamespace}, ownerEmptyObject).Return(&meta.NoKindMatchError{})

	service, err := s.Resolver.ResolvePodToServiceIdentity(context.Background(), &myPod)

	// should return the owner name as the service name
	s.Require().NoError(err)
	s.Require().Equal(ownerName, service.Name)
	s.Require().Equal("BadKind", service.Kind)

}

func (s *ServiceIdResolverTestSuite) TestServiceIdentityToPodLabelsForWorkloadSelection_DeploymentKind() {
	serviceName := "cool-service"
	namespace := "cool-namespace"
	service := serviceidentity.ServiceIdentity{Name: serviceName, Namespace: namespace, Kind: "Deployment"}

	s.Client.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.PodList{}), gomock.Eq(&client.ListOptions{Namespace: namespace}), map[string]string{
		v2alpha1.OtterizeServiceLabelKey:   service.GetFormattedOtterizeIdentityWithoutKind(),
		v2alpha1.OtterizeOwnerKindLabelKey: "Deployment",
	}).Do(func(_ any, podList *corev1.PodList, _ ...any) {
		podList.Items = append(podList.Items, corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "cool-pod", Namespace: namespace}})
	})

	pods, ok, err := s.Resolver.ResolveServiceIdentityToPodSlice(context.Background(), service)
	s.Require().NoError(err)
	s.Require().True(ok)
	s.Require().Len(pods, 1)
}

func (s *ServiceIdResolverTestSuite) TestServiceIdentityToPodLabelsForWorkloadSelection_ServiceKind() {
	serviceName := "cool-service"
	namespace := "cool-namespace"
	servicePodSelector := map[string]string{"cool-key": "cool-value"}
	service := serviceidentity.ServiceIdentity{Name: serviceName, Namespace: namespace, Kind: serviceidentity.KindService}

	serviceObj := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
		Spec:       corev1.ServiceSpec{Selector: servicePodSelector},
	}
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: serviceName, Namespace: namespace}, &corev1.Service{}).Do(func(_ context.Context, name types.NamespacedName, svc *corev1.Service, _ ...any) {
		serviceObj.DeepCopyInto(svc)
	})
	s.Client.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.PodList{}), gomock.Eq(&client.ListOptions{Namespace: namespace}), servicePodSelector).Do(func(_ any, podList *corev1.PodList, _ ...any) {
		podList.Items = append(podList.Items, corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "cool-pod", Namespace: namespace}})
	})

	pods, ok, err := s.Resolver.ResolveServiceIdentityToPodSlice(context.Background(), service)
	s.Require().NoError(err)
	s.Require().True(ok)
	s.Require().Len(pods, 1)
}

func (s *ServiceIdResolverTestSuite) TestServiceIdentityToPodLabelsForWorkloadSelection_ServiceKind_serviceNotFound() {
	serviceName := "cool-service"
	namespace := "cool-namespace"
	service := serviceidentity.ServiceIdentity{Name: serviceName, Namespace: namespace, Kind: serviceidentity.KindService}

	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: serviceName, Namespace: namespace}, &corev1.Service{}).Return(apierrors.NewNotFound(schema.GroupResource{Group: "", Resource: "services"}, serviceName))
	pods, ok, err := s.Resolver.ResolveServiceIdentityToPodSlice(context.Background(), service)
	s.Require().NoError(err)
	s.Require().False(ok)
	s.Require().Len(pods, 0)
}

func (s *ServiceIdResolverTestSuite) TestServiceIdentityToPodLabelsForWorkloadSelection_ServiceKind_podNotFound() {
	serviceName := "cool-service"
	namespace := "cool-namespace"
	servicePodSelector := map[string]string{"cool-key": "cool-value"}
	service := serviceidentity.ServiceIdentity{Name: serviceName, Namespace: namespace, Kind: serviceidentity.KindService}

	serviceObj := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: namespace},
		Spec:       corev1.ServiceSpec{Selector: servicePodSelector},
	}
	s.Client.EXPECT().Get(gomock.Any(), types.NamespacedName{Name: serviceName, Namespace: namespace}, &corev1.Service{}).Do(func(_ context.Context, name types.NamespacedName, svc *corev1.Service, _ ...any) {
		serviceObj.DeepCopyInto(svc)
	})
	// empty list
	s.Client.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.PodList{}), gomock.Eq(&client.ListOptions{Namespace: namespace}), servicePodSelector)

	pods, ok, err := s.Resolver.ResolveServiceIdentityToPodSlice(context.Background(), service)
	s.Require().NoError(err)
	s.Require().False(ok)
	s.Require().Len(pods, 0)
}

func (s *ServiceIdResolverTestSuite) TestUserSpecifiedAnnotationForServiceName() {
	annotationName := "coolAnnotationName"
	expectedEnvVarName := "OTTERIZE_WORKLOAD_NAME_OVERRIDE_ANNOTATION"
	_ = os.Setenv(expectedEnvVarName, annotationName)
	s.Require().Equal(annotationName, viper.GetString(podownerresolver.WorkloadNameOverrideAnnotationKey))
	_ = os.Unsetenv(expectedEnvVarName)
	s.Require().Equal(podownerresolver.WorkloadNameOverrideAnnotationKeyDefault, viper.GetString(podownerresolver.WorkloadNameOverrideAnnotationKey))
}

func TestServiceIdResolverTestSuite(t *testing.T) {
	suite.Run(t, new(ServiceIdResolverTestSuite))
}
