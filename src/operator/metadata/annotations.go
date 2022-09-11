package metadata

const (
	TLSSecretNameAnnotation = "spire-integration.otterize.com/tls-secret-name"
	DNSNamesAnnotation      = "spire-integration.otterize.com/dns-names"
	CertTTLAnnotation       = "spire-integration.otterize.com/cert-ttl"
	CertTypeAnnotation      = "spire-integration.otterize.com/cert-type"
	JksPasswordAnnotation   = "spire-integration.otterize.com/jks-password"

	SVIDFileNameAnnotation       = "spire-integration.otterize.com/svid-file-name"
	BundleFileNameAnnotation     = "spire-integration.otterize.com/bundle-file-name"
	KeyFileNameAnnotation        = "spire-integration.otterize.com/key-file-name"
	KeyStoreFileNameAnnotation   = "spire-integration.otterize.com/keystore-file-name"
	TrustStoreFileNameAnnotation = "spire-integration.otterize.com/truststore-file-name"

	TLSSecretServiceNameAnnotation = "spire-integration.otterize.com/service-name"
	TLSSecretEntryIDAnnotation     = "spire-integration.otterize.com/entry-id"
	TLSSecretEntryHashAnnotation   = "spire-integration.otterize.com/entry-hash"
	TLSSecretSVIDExpiryAnnotation  = "spire-integration.otterize.com/svid-expires-at"
)
