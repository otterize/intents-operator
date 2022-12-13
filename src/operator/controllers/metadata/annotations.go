package metadata

// User input annotations, to be used by users to specify tls certificates settings
const (
	// TLSSecretNameAnnotation is the name of the k8s secret in which the certificate data is stored
	TLSSecretNameAnnotation = "spire-integration.otterize.com/tls-secret-name"

	// DNSNamesAnnotation is a comma-separated list of additional dns names to be registered as part of the
	// SPIRE-server entry and encoded into the certificate data
	DNSNamesAnnotation = "spire-integration.otterize.com/dns-names"

	// CertTTLAnnotation is the certificate TTL. Defaults to SPIRE-server's configured default TTL.
	CertTTLAnnotation = "spire-integration.otterize.com/cert-ttl"

	// CertTypeAnnotation is the requested certificate type - pem (default) or jks.
	CertTypeAnnotation = "spire-integration.otterize.com/cert-type"

	// ShouldRestartOnRenewalAnnotation - After a certificate is being renewed, pods with this annotation set to "true"
	// will be restarted. Defaults to "false"
	ShouldRestartOnRenewalAnnotation = "spire-integration.otterize.com/restart-pod-on-certificate-renewal"

	// SVIDFileNameAnnotation holds the name of the file in the secret data, that stores the certificate's SVID file
	// (for pem certificate type). Defaults to "svid.pem".
	SVIDFileNameAnnotation = "spire-integration.otterize.com/svid-file-name"
	// BundleFileNameAnnotation holds the name of the file in the secret data, that stores the certificate's bundle file
	// (for pem certificate type). Defaults to "bundle.pem".
	BundleFileNameAnnotation = "spire-integration.otterize.com/bundle-file-name"
	// KeyFileNameAnnotation holds the name of the file in the secret data, that stores the certificate's key file
	// (for pem certificate type). Defaults to "key.pem".
	KeyFileNameAnnotation = "spire-integration.otterize.com/key-file-name"

	// KeyStoreFileNameAnnotation holds the name of the file in the secret data, that stores the certificate's key store file
	// (for jks certificate type). Defaults to "keystore.jks".
	KeyStoreFileNameAnnotation = "spire-integration.otterize.com/keystore-file-name"
	// TrustStoreFileNameAnnotation holds the name of the file in the secret data, that stores the certificate's trust store file
	// (for jks certificate type). Defaults to "truststore.jks".
	TrustStoreFileNameAnnotation = "spire-integration.otterize.com/truststore-file-name"
	// JKSPasswordAnnotation is the jks certificate password (for jks certificate type). Defaults to "password".
	JKSPasswordAnnotation = "spire-integration.otterize.com/jks-password"
)

// Internal use annotations, used by the operator to store data on top of generated k8s secrets.
const (
	// TLSSecretRegisteredServiceNameAnnotation is the registered service name of the service for which
	// this secret is used.
	TLSSecretRegisteredServiceNameAnnotation = "spire-integration.otterize.com/registered-service-name"
	// TLSSecretEntryIDAnnotation is the SPIRE-server entry id using which this secret was generated.
	TLSSecretEntryIDAnnotation = "spire-integration.otterize.com/entry-id"
	// TLSSecretEntryHashAnnotation is the hash of the SPIRE-server entry using which this secret was generated.
	TLSSecretEntryHashAnnotation = "spire-integration.otterize.com/entry-hash"
	// TLSSecretSVIDExpiryAnnotation is the expiry time of the encoded svid, used to determine when this secret's data
	// should be refreshed.
	TLSSecretSVIDExpiryAnnotation = "spire-integration.otterize.com/svid-expires-at"
	// TLSRestartTimeAfterRenewal is the last time the owner's pods were restarted due to a secret being refrshed
	TLSRestartTimeAfterRenewal = "spire-integration.otterize.com/restart-time-after-renewal"
)
