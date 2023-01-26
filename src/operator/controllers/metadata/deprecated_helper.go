package metadata

import "github.com/samber/lo"

func annotationNameToDeprecatedName() map[string]string {
	return map[string]string{
		JKSPasswordAnnotation:            JKSPasswordAnnotationDeprecated,
		TrustStoreFileNameAnnotation:     TrustStoreFileNameAnnotationDeprecated,
		KeyStoreFileNameAnnotation:       KeyStoreFileNameAnnotationDeprecated,
		KeyFileNameAnnotation:            KeyFileNameAnnotationDeprecated,
		CAFileNameAnnotation:             BundleFileNameAnnotationDeprecated,
		CertFileNameAnnotation:           SVIDFileNameAnnotationDeprecated,
		ShouldRestartOnRenewalAnnotation: ShouldRestartOnRenewalAnnotationDeprecated,
		CertTypeAnnotation:               CertTypeAnnotationDeprecated,
		CertTTLAnnotation:                CertTTLAnnotationDeprecated,
		DNSNamesAnnotation:               DNSNamesAnnotationDeprecated,
		TLSSecretNameAnnotation:          TLSSecretNameAnnotationDeprecated,
	}
}

func GetAnnotationValue(annotations map[string]string, name string) string {
	deprecatedName, ok := annotationNameToDeprecatedName()[name]
	if ok {
		res, _ := lo.Coalesce(annotations[name], annotations[deprecatedName])
		return res
	}
	return annotations[name]
}

func AnnotationExists(annotations map[string]string, name string) bool {
	deprecatedName, ok := annotationNameToDeprecatedName()[name]
	_, exists := annotations[name]
	if !ok {
		return exists
	}
	_, existsDeprecated := annotations[deprecatedName]
	return exists || existsDeprecated
}

func HasDeprecatedAnnotations(annotation map[string]string) bool {
	return len(lo.Intersect(lo.Keys(annotation), lo.Values(annotationNameToDeprecatedName()))) != 0
}
