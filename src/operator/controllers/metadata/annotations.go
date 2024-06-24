package metadata

// User input annotations, to be used by users to specify tls certificates settings
const (

	// UserAndPasswordSecretNameAnnotation is the name of the secret in which the user and password are stored
	UserAndPasswordSecretNameAnnotation = "credentials-operator.otterize.com/user-password-secret-name"

	// RestartOnSecretRotation signals the
	RestartOnSecretRotation = "credentials-operator.otterize.com/restart-on-secret-rotation"

	// TLSSecretNameAnnotation is the name of the K8s secret in which the certificate data is stored
	TLSSecretNameAnnotation           = "credentials-operator.otterize.com/tls-secret-name"
	TLSSecretNameAnnotationDeprecated = "spire-integration.otterize.com/tls-secret-name"

	// DeprecatedIAMRoleFinalizer indicates that cleanup on IAM roles is needed upon termination.
	// Deprecated: replaced by AWSAgentFinalizer, GCPAgentFinalizer, and AzureAgentFinalizer
	DeprecatedIAMRoleFinalizer = "credentials-operator.otterize.com/iam-role"

	// DNSNamesAnnotation is a comma-separated list of additional dns names to be registered as part of the
	// SPIRE-server entry and encoded into the certificate data
	DNSNamesAnnotation           = "credentials-operator.otterize.com/dns-names"
	DNSNamesAnnotationDeprecated = "spire-integration.otterize.com/dns-names"

	// CertTTLAnnotation is the certificate TTL. Defaults to server's configured default TTL.
	CertTTLAnnotation           = "credentials-operator.otterize.com/cert-ttl"
	CertTTLAnnotationDeprecated = "spire-integration.otterize.com/cert-ttl"

	// CertTypeAnnotation is the requested certificate type - pem (default) or jks.
	CertTypeAnnotation           = "credentials-operator.otterize.com/cert-type"
	CertTypeAnnotationDeprecated = "spire-integration.otterize.com/cert-type"

	// ShouldRestartOnRenewalAnnotation - After a certificate is being renewed, pods with this annotation,
	// will be restarted. Defaults to "false"
	ShouldRestartOnRenewalAnnotation           = "credentials-operator.otterize.com/restart-pod-on-certificate-renewal"
	ShouldRestartOnRenewalAnnotationDeprecated = "spire-integration.otterize.com/restart-pod-on-certificate-renewal"

	// CertFileNameAnnotation holds the name of the file in the secret data, that stores the certificate's Certificate file
	// (for pem certificate type). Defaults to "cert.pem".
	CertFileNameAnnotation           = "credentials-operator.otterize.com/cert-file-name"
	SVIDFileNameAnnotationDeprecated = "spire-integration.otterize.com/svid-file-name"
	// CAFileNameAnnotation holds the name of the file in the secret data, that stores the ca bundle file
	// (for pem certificate type). Defaults to "ca.pem".
	CAFileNameAnnotation               = "credentials-operator.otterize.com/ca-file-name"
	BundleFileNameAnnotationDeprecated = "spire-integration.otterize.com/bundle-file-name"
	// KeyFileNameAnnotation holds the name of the file in the secret data, that stores the certificate's key file
	// (for pem certificate type). Defaults to "key.pem".
	KeyFileNameAnnotation           = "credentials-operator.otterize.com/key-file-name"
	KeyFileNameAnnotationDeprecated = "spire-integration.otterize.com/key-file-name"

	// KeyStoreFileNameAnnotation holds the name of the file in the secret data, that stores the certificate's key store file
	// (for jks certificate type). Defaults to "keystore.jks".
	KeyStoreFileNameAnnotation           = "credentials-operator.otterize.com/keystore-file-name"
	KeyStoreFileNameAnnotationDeprecated = "spire-integration.otterize.com/keystore-file-name"
	// TrustStoreFileNameAnnotation holds the name of the file in the secret data, that stores the certificate's trust store file
	// (for jks certificate type). Defaults to "truststore.jks".
	TrustStoreFileNameAnnotation           = "credentials-operator.otterize.com/truststore-file-name"
	TrustStoreFileNameAnnotationDeprecated = "spire-integration.otterize.com/truststore-file-name"
	// JKSPasswordAnnotation is the jks certificate password (for jks certificate type). Defaults to "password".
	JKSPasswordAnnotation           = "credentials-operator.otterize.com/jks-password"
	JKSPasswordAnnotationDeprecated = "spire-integration.otterize.com/jks-password"
)

// Internal use annotations, used by the operator to store data on top of generated k8s secrets.
const (
	// TLSSecretRegisteredServiceNameAnnotation is the registered service name of the service for which
	// this secret is used.
	TLSSecretRegisteredServiceNameAnnotation = "credentials-operator.otterize.com/registered-service-name"
	// TLSSecretEntryIDAnnotation is the SPIRE-server entry id using which this secret was generated.
	TLSSecretEntryIDAnnotation = "credentials-operator.otterize.com/entry-id"
	// TLSSecretEntryHashAnnotation is the hash of the SPIRE-server entry using which this secret was generated.
	TLSSecretEntryHashAnnotation = "credentials-operator.otterize.com/entry-hash"
	// TLSSecretExpiryAnnotation is the expiry time of the encoded svid, used to determine when this secret's data
	// should be refreshed.
	TLSSecretExpiryAnnotation = "credentials-operator.otterize.com/svid-expires-at"
	// TLSRestartTimeAfterRenewal is the last time the owner's pods were restarted due to a secret being refrshed
	TLSRestartTimeAfterRenewal = "credentials-operator.otterize.com/restart-time-after-renewal"
	// SecretLastUpdatedTimestampAnnotation represents the last time the secret data was updated, used for rotating passwords
	SecretLastUpdatedTimestampAnnotation = "credentials-operator.otterize.com/last-updated-timestamp"
)
