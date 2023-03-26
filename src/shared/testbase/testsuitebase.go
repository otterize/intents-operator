package testbase

import (
	"context"
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	_ "os"
	ctrl "sigs.k8s.io/controller-runtime"
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

func (s *ControllerManagerTestSuiteBase) AddPod(name string, podIp string, labels map[string]string, annotations map[string]string) {
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
	s.waitForObjectToBeCreated(pod)

	if podIp != "" {
		pod.Status.PodIP = podIp
		pod.Status.PodIPs = []corev1.PodIP{{IP: podIp}}
		_, err = s.K8sDirectClient.CoreV1().Pods(s.TestNamespace).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{})
		s.Require().NoError(err)
	}
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
		podName := fmt.Sprintf("%s-%d", name, i)
		s.AddPod(podName, ip, podLabels, annotations)
		s.WaitUntilCondition(func(assert *assert.Assertions) {
			pod := corev1.Pod{}
			err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Name: podName, Namespace: s.TestNamespace}, &pod)
			s.Require().NoError(err)

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
			err = s.Mgr.GetClient().Update(context.Background(), &pod)
			assert.NoError(err)
		})
	}

	return replicaSet
}

type Reconciler interface {
	Reconcile(context.Context, ctrl.Request) (ctrl.Result, error)
}

func (s *ControllerManagerTestSuiteBase) RunReconciler(reconciler Reconciler, namespacedName types.NamespacedName) {
	res := ctrl.Result{Requeue: true}
	var err error

	for res.Requeue {
		res, err = reconciler.Reconcile(context.Background(), ctrl.Request{
			NamespacedName: namespacedName,
		})
	}

	s.Require().NoError(err)
	s.Require().Empty(res)
	s.Require().True(s.Mgr.GetCache().WaitForCacheSync(context.Background()))
}

func (s *ControllerManagerTestSuiteBase) AddDeployment(
	name string,
	podIps []string,
	podLabels map[string]string,
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

	s.WaitUntilCondition(func(assert *assert.Assertions) {
		rs := &appsv1.ReplicaSet{}
		err = s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{
			Namespace: s.TestNamespace, Name: replicaSet.GetName()}, rs)
		assert.NoError(err)
		assert.NotEmpty(replicaSet.OwnerReferences)
	})

	return deployment
}

func (s *ControllerManagerTestSuiteBase) AddEndpoints(name string, podIps []string) *corev1.Endpoints {
	podIpsSet := sets.NewString(podIps...)
	podList := &corev1.PodList{}
	err := s.Mgr.GetClient().List(context.Background(), podList, client.InNamespace(s.TestNamespace))
	s.Require().NoError(err)

	addresses := lo.FilterMap(podList.Items, func(pod corev1.Pod, _ int) (corev1.EndpointAddress, bool) {
		if !podIpsSet.Has(pod.Status.PodIP) {
			return corev1.EndpointAddress{}, false
		}
		return corev1.EndpointAddress{
			IP: pod.Status.PodIP,
			TargetRef: &corev1.ObjectReference{
				Kind:            "Pod",
				Name:            pod.Name,
				Namespace:       pod.Namespace,
				UID:             pod.UID,
				ResourceVersion: pod.ResourceVersion,
			},
		}, true
	})

	endpoints := &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.TestNamespace},
		Subsets:    []corev1.EndpointSubset{{Addresses: addresses, Ports: []corev1.EndpointPort{{Name: "someport", Port: 8080, Protocol: corev1.ProtocolTCP}}}},
	}

	err = s.Mgr.GetClient().Create(context.Background(), endpoints)
	s.Require().NoError(err)

	s.waitForObjectToBeCreated(endpoints)
	return endpoints
}

func (s *ControllerManagerTestSuiteBase) AddService(name string, podIps []string, selector map[string]string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.TestNamespace},
		Spec: corev1.ServiceSpec{Selector: selector,
			Ports: []corev1.ServicePort{{Name: "someport", Port: 8080, Protocol: corev1.ProtocolTCP}},
			Type:  corev1.ServiceTypeClusterIP,
		},
	}
	err := s.Mgr.GetClient().Create(context.Background(), service)
	s.Require().NoError(err)

	s.waitForObjectToBeCreated(service)

	s.AddEndpoints(name, podIps)
	return service
}

func (s *ControllerManagerTestSuiteBase) AddIngress(serviceName string) *networkingv1.Ingress {
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: serviceName + "-ingress", Namespace: s.TestNamespace},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: serviceName + ".test.domain",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: lo.ToPtr(networkingv1.PathTypePrefix),
									Backend:  networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: serviceName, Port: networkingv1.ServiceBackendPort{Number: 80}}},
								}},
						},
					},
				},
			},
		},
	}
	err := s.Mgr.GetClient().Create(context.Background(), ingress)
	s.Require().NoError(err)

	s.waitForObjectToBeCreated(ingress)

	return ingress
}

func (s *ControllerManagerTestSuiteBase) AddLoadBalancerService(name string, podIps []string, selector map[string]string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.TestNamespace},
		Spec: corev1.ServiceSpec{Selector: selector,
			Ports: []corev1.ServicePort{{Name: "someport", Port: 8080, Protocol: corev1.ProtocolTCP}},
			Type:  corev1.ServiceTypeLoadBalancer,
		},
	}
	err := s.Mgr.GetClient().Create(context.Background(), service)
	s.Require().NoError(err)

	s.waitForObjectToBeCreated(service)

	s.AddEndpoints(name, podIps)
	return service
}

func (s *ControllerManagerTestSuiteBase) AddNodePortService(name string, podIps []string, selector map[string]string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.TestNamespace},
		Spec: corev1.ServiceSpec{Selector: selector,
			Ports: []corev1.ServicePort{{Name: "someport", Port: 8080, Protocol: corev1.ProtocolTCP}},
			Type:  corev1.ServiceTypeNodePort,
		},
	}
	err := s.Mgr.GetClient().Create(context.Background(), service)
	s.Require().NoError(err)

	s.waitForObjectToBeCreated(service)

	s.AddEndpoints(name, podIps)
	return service
}

func (s *ControllerManagerTestSuiteBase) AddDeploymentWithService(name string, podIps []string, podLabels map[string]string, podAnnotations map[string]string) (*appsv1.Deployment, *corev1.Service) {
	deployment := s.AddDeployment(name, podIps, podLabels, podAnnotations)
	service := s.AddService(name, podIps, podLabels)
	return deployment, service
}

func (s *ControllerManagerTestSuiteBase) AddKafkaServerConfig(kafkaServerConfig *otterizev1alpha2.KafkaServerConfig) {
	err := s.Mgr.GetClient().Create(context.Background(), kafkaServerConfig)
	s.Require().NoError(err)

	s.waitForObjectToBeCreated(kafkaServerConfig)
}

func (s *ControllerManagerTestSuiteBase) RemoveKafkaServerConfig(objName string) {
	kafkaServerConfig := &otterizev1alpha2.KafkaServerConfig{}
	err := s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Name: objName, Namespace: s.TestNamespace}, kafkaServerConfig)
	s.Require().NoError(err)

	err = s.Mgr.GetClient().Delete(context.Background(), kafkaServerConfig)
	s.Require().NoError(err)

	s.WaitForDeletionToBeMarked(kafkaServerConfig)
}

func (s *ControllerManagerTestSuiteBase) AddIntents(
	objName,
	clientName string,
	callList []otterizev1alpha2.Intent) (*otterizev1alpha2.ClientIntents, error) {

	intents := &otterizev1alpha2.ClientIntents{
		ObjectMeta: metav1.ObjectMeta{Name: objName, Namespace: s.TestNamespace},
		Spec: &otterizev1alpha2.IntentsSpec{
			Service: otterizev1alpha2.Service{Name: clientName},
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

func (s *ControllerManagerTestSuiteBase) UpdateIntents(
	objName string,
	callList []otterizev1alpha2.Intent) error {

	intents := &otterizev1alpha2.ClientIntents{}
	err := s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Name: objName, Namespace: s.TestNamespace}, intents)
	s.Require().NoError(err)

	intents.Spec.Calls = callList

	return s.Mgr.GetClient().Update(context.Background(), intents)
}

func (s *ControllerManagerTestSuiteBase) RemoveIntents(
	objName string) error {

	intents := &otterizev1alpha2.ClientIntents{}
	err := s.Mgr.GetClient().Get(context.Background(), types.NamespacedName{Name: objName, Namespace: s.TestNamespace}, intents)
	if err != nil {
		return err
	}

	err = s.Mgr.GetClient().Delete(context.Background(), intents)
	if err != nil {
		return err
	}

	s.WaitForDeletionToBeMarked(intents)

	return nil
}
