package external_traffic

import (
	"context"
	"fmt"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const OtterizeNetworkPolicyNameTemplate = "external-access-to-%s-from-%s"
const OtterizeNetworkPolicy = "otterize/network-policy"

type ServiceReconciler struct {
	client            client.Client
	Scheme            *runtime.Scheme
	ingressReconciler *IngressReconciler
	injectablerecorder.InjectableRecorder
}

func formatPolicyName(name string, namespace string) string {
	return fmt.Sprintf(OtterizeNetworkPolicyNameTemplate, name, namespace)
}

func NewServiceReconciler(client client.Client, scheme *runtime.Scheme, ingressReconciler *IngressReconciler) *ServiceReconciler {
	return &ServiceReconciler{
		client:            client,
		Scheme:            scheme,
		ingressReconciler: ingressReconciler,
	}
}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.InjectRecorder(mgr.GetEventRecorderFor("intents-operator"))

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}

func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	res, err := r.ingressReconciler.ReconcileService(ctx, req)
	if err != nil || !res.IsZero() {
		return res, err
	}

	svc := &corev1.Service{}
	err = r.client.Get(ctx, req.NamespacedName, svc)
	if k8serrors.IsNotFound(err) {
		// delete is handled by garbage collection - the service owns the network policy
		return ctrl.Result{}, nil
	}

	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer && svc.Spec.Type != corev1.ServiceTypeNodePort {
		return ctrl.Result{}, nil
	}
	err = r.handleNetworkPolicyCreationOrUpdate(ctx, svc)
	if err != nil {
		if k8serrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) handleNetworkPolicyCreationOrUpdate(
	ctx context.Context, service *corev1.Service) error {

	policyName := formatPolicyName(service.Name, service.Namespace)
	existingPolicy := &v1.NetworkPolicy{}
	err := r.client.Get(ctx, types.NamespacedName{Name: policyName, Namespace: service.Namespace}, existingPolicy)
	newPolicy := r.buildNetworkPolicyObjectForService(service, policyName)
	newPolicy.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: service.APIVersion,
		Kind:       service.Kind,
		Name:       service.Name,
		UID:        service.UID,
	}})

	// No matching network policy found, create one
	if k8serrors.IsNotFound(err) {
		logrus.Infof(
			"Creating network policy to enable access from external traffic to load balancer service %s (ns %s)", service.Name, service.Namespace)
		err := r.client.Create(ctx, newPolicy)
		if err != nil {
			r.RecordWarningEvent(service, "failed to create external traffic network policy", err.Error())
			return err
		}
		return nil

	} else if err != nil {
		r.RecordWarningEvent(service, "failed to get external traffic network policy", err.Error())
		return err
	}

	// Found matching policy, is an update needed?
	if reflect.DeepEqual(existingPolicy.Spec, newPolicy.Spec) {
		return nil
	}

	policyCopy := existingPolicy.DeepCopy()
	policyCopy.Spec = newPolicy.Spec
	policyCopy.SetOwnerReferences(newPolicy.GetOwnerReferences())

	err = r.client.Patch(ctx, policyCopy, client.MergeFrom(existingPolicy))
	if err != nil {
		return err
	}

	return nil
}

func (r *ServiceReconciler) buildNetworkPolicyObjectForService(
	service *corev1.Service, policyName string) *v1.NetworkPolicy {
	serviceSpecCopy := service.Spec.DeepCopy()

	netpol := &v1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: service.Namespace,
			Labels: map[string]string{
				OtterizeNetworkPolicy: "true",
			},
		},
		Spec: v1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: serviceSpecCopy.Selector,
			},
			Ingress: []v1.NetworkPolicyIngressRule{
				{},
			},
		},
	}

	for _, port := range serviceSpecCopy.Ports {
		netpolPort := v1.NetworkPolicyPort{
			Port: lo.ToPtr(port.TargetPort),
		}

		if port.TargetPort.IntVal == 0 && len(port.TargetPort.StrVal) == 0 {
			netpolPort.Port = lo.ToPtr(intstr.FromInt(int(port.Port)))
		}

		if len(port.Protocol) != 0 {
			netpolPort.Protocol = lo.ToPtr(port.Protocol)
		}
		netpol.Spec.Ingress[0].Ports = append(netpol.Spec.Ingress[0].Ports, netpolPort)
	}

	return netpol
}
