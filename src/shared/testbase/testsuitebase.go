package testbase

import (
	"context"
	"fmt"
	otterizev1alpha1 "github.com/otterize/intents-operator/src/operator/api/v1alpha1"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	_ "os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"strings"
	"testing"
	"time"
)

const waitForCreationInterval = 20 * time.Millisecond
const waitForCreationTimeout = 10 * time.Second
const waitForDeletionTSTimeout = 3 * time.Second

type ControllerManagerTestSuiteBase struct {
	suite.Suite
	TestEnv          *envtest.Environment
	RestConfig       *rest.Config
	TestNamespace    string
	K8sDirectClient  *kubernetes.Clientset
	mgrCtx           context.Context
	mgrCtxCancelFunc context.CancelFunc
	Mgr              manager.Manager
}

func (s *ControllerManagerTestSuiteBase) TearDownSuite() {
	s.Require().NoError(s.TestEnv.Stop())
}

func (s *ControllerManagerTestSuiteBase) SetupTest() {
	s.mgrCtx, s.mgrCtxCancelFunc = context.WithCancel(context.Background())

	var err error
	s.Mgr, err = manager.New(s.RestConfig, manager.Options{MetricsBindAddress: "0"})
	s.Require().NoError(err)
}

// BeforeTest happens AFTER the SetupTest()
func (s *ControllerManagerTestSuiteBase) BeforeTest(_, testName string) {
	go func() {
		// We start the manager in "Before test" to allow operations that should happen before start to be run at SetupTest()
		err := s.Mgr.Start(s.mgrCtx)
		s.Require().NoError(err)
	}()

	s.TestNamespace = strings.ToLower(fmt.Sprintf("%s-%s", testName, time.Now().Format("20060102150405")))
	testNamespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: s.TestNamespace},
	}
	_, err := s.K8sDirectClient.CoreV1().Namespaces().Create(context.Background(), testNamespaceObj, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *ControllerManagerTestSuiteBase) TearDownTest() {
	s.mgrCtxCancelFunc()
	err := s.K8sDirectClient.CoreV1().Namespaces().Delete(context.Background(), s.TestNamespace, metav1.DeleteOptions{})
	s.Require().NoError(err)
}

type Condition func() bool

func (s *ControllerManagerTestSuiteBase) WaitUntilCondition(cond func(assert *assert.Assertions)) {
	err := wait.PollImmediate(waitForCreationInterval, waitForCreationTimeout, func() (done bool, err error) {
		localT := &testing.T{}
		asrt := assert.New(localT)
		s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
		cond(asrt)
		done = !localT.Failed()
		return done, nil
	})
	if err != nil {
		s.Require().NoError(err)
	}
}

// waitForObjectToBeCreated tries to get an object multiple times until it is available in the k8s API server
func (s *ControllerManagerTestSuiteBase) waitForObjectToBeCreated(obj client.Object) {
	s.Require().NoError(wait.PollImmediate(waitForCreationInterval, waitForCreationTimeout, func() (done bool, err error) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, obj)
		if errors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}))
}

// WaitForDeletionToBeMarked tries to get an object multiple times until its deletion timestamp is set in K8S
func (s *ControllerManagerTestSuiteBase) WaitForDeletionToBeMarked(obj client.Object) {
	s.Require().NoError(wait.PollImmediate(waitForCreationInterval, waitForDeletionTSTimeout, func() (done bool, err error) {
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, obj)
		if !obj.GetDeletionTimestamp().IsZero() {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	}))
}

func (s *ControllerManagerTestSuiteBase) AddPod(name string, podIp string, labels, annotations map[string]string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.TestNamespace, Labels: labels, Annotations: annotations},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{
				Name:            name,
				Image:           "nginx",
				ImagePullPolicy: "Always",
			},
		},
		},
	}
	err := s.Mgr.GetClient().Create(context.Background(), pod)
	s.Require().NoError(err)

	if podIp != "" {
		pod.Status.PodIP = podIp
		pod.Status.PodIPs = []corev1.PodIP{{podIp}}
		pod, err = s.K8sDirectClient.CoreV1().Pods(s.TestNamespace).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{})
		s.Require().NoError(err)
	}
	s.waitForObjectToBeCreated(pod)
	return pod
}

func (s *ControllerManagerTestSuiteBase) AddReplicaSet(
	name string,
	podIps []string,
	podLabels,
	annotations map[string]string) *appsv1.ReplicaSet {
	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.TestNamespace},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: lo.ToPtr(int32(len(podIps))),
			Selector: &metav1.LabelSelector{MatchLabels: podLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.TestNamespace, Labels: podLabels, Annotations: annotations},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{
						Name:            name,
						Image:           "nginx",
						ImagePullPolicy: "Always",
					},
				},
				},
			},
		},
	}
	err := s.Mgr.GetClient().Create(context.Background(), replicaSet)
	s.Require().NoError(err)

	s.waitForObjectToBeCreated(replicaSet)

	for i, ip := range podIps {
		pod := s.AddPod(fmt.Sprintf("%s-%d", name, i), ip, podLabels, annotations)
		pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         "apps/v1",
				Kind:               "ReplicaSet",
				BlockOwnerDeletion: lo.ToPtr(true),
				Controller:         lo.ToPtr(true),
				Name:               replicaSet.Name,
				UID:                replicaSet.UID,
			},
		}
		err := s.Mgr.GetClient().Update(context.Background(), pod)
		s.Require().NoError(err)
	}

	return replicaSet
}

func (s *ControllerManagerTestSuiteBase) AddDeployment(
	name string,
	podIps []string,
	podLabels,
	annotations map[string]string) *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.TestNamespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: lo.ToPtr(int32(len(podIps))),
			Selector: &metav1.LabelSelector{MatchLabels: podLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.TestNamespace, Labels: podLabels, Annotations: annotations},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{
						Name:            name,
						Image:           "nginx",
						ImagePullPolicy: "Always",
					},
				},
				},
			},
		},
	}
	err := s.Mgr.GetClient().Create(context.Background(), deployment)
	s.Require().NoError(err)

	s.waitForObjectToBeCreated(deployment)

	replicaSet := s.AddReplicaSet(name, podIps, podLabels, annotations)
	replicaSet.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion:         "apps/v1",
			Kind:               "Deployment",
			BlockOwnerDeletion: lo.ToPtr(true),
			Controller:         lo.ToPtr(true),
			Name:               deployment.Name,
			UID:                deployment.UID,
		},
	}
	err = s.Mgr.GetClient().Update(context.Background(), replicaSet)
	s.Require().NoError(err)
	s.waitForObjectToBeCreated(replicaSet)

	return deployment
}

func (s *ControllerManagerTestSuiteBase) AddIntents(
	objName,
	clientName string,
	callList []otterizev1alpha1.Intent) (*otterizev1alpha1.ClientIntents, error) {

	intents := &otterizev1alpha1.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{Name: objName, Namespace: s.TestNamespace},
		Spec: &otterizev1alpha1.IntentsSpec{
			Service: otterizev1alpha1.Service{Name: clientName},
			Calls:   callList,
		},
	}
	err := s.Mgr.GetClient().Create(context.Background(), intents)
	if err != nil {
		return nil, err
	}
	s.waitForObjectToBeCreated(intents)

	return intents, nil
}
