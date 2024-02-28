package filters

import (
	"github.com/otterize/intents-operator/src/shared"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const OtterizeLabelKey = "app.kubernetes.io/part-of"
const OtterizeLabelValue = "otterize"
const IntentsOperatorLabelKey = "app.kubernetes.io/component"
const IntentsOperatorLabelValue = "intents-operator"
const CredentialsOperatorLabelKey = "app.kubernetes.io/component"
const CredentialsOperatorLabelValue = "credentials-operator"
const NetworkMapperLabelKey = "app.kubernetes.io/component"
const NetworkMapperLabelValue = "network-mapper"

func IntentsOperatorLabels() map[string]string {
	return map[string]string{
		OtterizeLabelKey:        OtterizeLabelValue,
		IntentsOperatorLabelKey: IntentsOperatorLabelValue,
	}
}

func CredentialsOperatorLabels() map[string]string {
	return map[string]string{
		OtterizeLabelKey:            OtterizeLabelValue,
		CredentialsOperatorLabelKey: CredentialsOperatorLabelValue,
	}
}

func NetworkMapperLabels() map[string]string {
	return map[string]string{
		OtterizeLabelKey:      OtterizeLabelValue,
		NetworkMapperLabelKey: NetworkMapperLabelValue,
	}
}

func PartOfOtterizeLabels() map[string]string {
	return map[string]string{
		OtterizeLabelKey: OtterizeLabelValue,
	}
}

func IntentsOperatorLabelPredicate() predicate.Predicate {
	return shared.MustRet(predicate.LabelSelectorPredicate(metav1.LabelSelector{MatchLabels: IntentsOperatorLabels()}))
}

func CredentialsOperatorLabelPredicate() predicate.Predicate {
	return shared.MustRet(predicate.LabelSelectorPredicate(metav1.LabelSelector{MatchLabels: CredentialsOperatorLabels()}))
}

func NetworkMapperLabelPredicate() predicate.Predicate {
	return shared.MustRet(predicate.LabelSelectorPredicate(metav1.LabelSelector{MatchLabels: NetworkMapperLabels()}))
}

func PartOfOtterizeLabelPredicate() predicate.Predicate {
	return shared.MustRet(predicate.LabelSelectorPredicate(metav1.LabelSelector{MatchLabels: PartOfOtterizeLabels()}))
}
