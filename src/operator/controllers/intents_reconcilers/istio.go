package intents_reconcilers

import (
	"fmt"
	otterizev1alpha2 "github.com/otterize/intents-operator/src/operator/api/v1alpha2"
	"github.com/sirupsen/logrus"
	specapi "istio.io/api/security/v1beta1"
	istiocommon "istio.io/api/type/v1beta1"
	"istio.io/client-go/pkg/apis/security/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateAuthorizationPolicy(
	intent otterizev1alpha2.Intent,
	intentsObjNamespace string,
	policyName string,
	clientServiceAccountName string,
) *v1beta1.AuthorizationPolicy {
	logrus.Info("CreateAuthorizationPolicy istio client start")

	serverNamespace := intent.GetServerNamespace(intentsObjNamespace)
	formattedTargetServer := otterizev1alpha2.GetFormattedOtterizeIdentity(intent.GetServerName(), serverNamespace)

	// TODO: make trust domain configurable
	source := fmt.Sprintf("cluster.local/ns/%s/sa/%s", intentsObjNamespace, clientServiceAccountName)
	ap := v1beta1.AuthorizationPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name:      policyName,
			Namespace: serverNamespace,
		},
		Spec: specapi.AuthorizationPolicy{
			Selector: &istiocommon.WorkloadSelector{
				MatchLabels: map[string]string{
					otterizev1alpha2.OtterizeServerLabelKey: formattedTargetServer,
				},
			},
			Action: specapi.AuthorizationPolicy_ALLOW,
			Rules: []*specapi.Rule{
				{
					From: []*specapi.Rule_From{
						{
							Source: &specapi.Source{
								Principals: []string{
									source,
								},
							},
						},
					},
				},
			},
		},
	}

	return &ap
}
