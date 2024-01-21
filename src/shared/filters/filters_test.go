package filters

import (
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterByOtterizeLabel(t *testing.T) {
	obj := &admissionregistrationv1.ValidatingWebhookConfiguration{}

	result := filterByOtterizeLabel(obj)
	assert.False(t, result)

	// Set the label value to "Otterize"
	labels := map[string]string{labelKey: labelValue}
	obj.SetLabels(labels)

	result = filterByOtterizeLabel(obj)
	assert.True(t, result)
}
