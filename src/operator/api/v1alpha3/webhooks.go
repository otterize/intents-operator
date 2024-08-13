package v1alpha3

import (
	"github.com/otterize/intents-operator/src/operator/api/v2alpha1"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// MySQLServerConfig //

func (in *MySQLServerConfig) SetupWebhookWithManager(mgr ctrl.Manager, validator webhook.CustomValidator) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).WithValidator(validator).
		Complete()
}

func (in *MySQLServerConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v2alpha1.MySQLServerConfig)
	dst.ObjectMeta = in.ObjectMeta
	dst.Spec.Address = in.Spec.Address
	dst.Spec.Credentials.Username = in.Spec.Credentials.Username
	dst.Spec.Credentials.Password = in.Spec.Credentials.Password
	if in.Spec.Credentials.SecretRef != nil {
		dst.Spec.Credentials.SecretRef = &v2alpha1.DatabaseCredentialsSecretRef{
			Name:        in.Spec.Credentials.SecretRef.Name,
			Namespace:   in.Spec.Credentials.SecretRef.Namespace,
			UsernameKey: in.Spec.Credentials.SecretRef.UsernameKey,
			PasswordKey: in.Spec.Credentials.SecretRef.PasswordKey,
		}
	}
	return nil
}

func (in *MySQLServerConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2alpha1.MySQLServerConfig)
	in.ObjectMeta = src.ObjectMeta
	in.Spec.Address = src.Spec.Address
	in.Spec.Credentials.Username = src.Spec.Credentials.Username
	in.Spec.Credentials.Password = src.Spec.Credentials.Password
	if src.Spec.Credentials.SecretRef != nil {
		in.Spec.Credentials.SecretRef = &DatabaseCredentialsSecretRef{
			Name:        src.Spec.Credentials.SecretRef.Name,
			Namespace:   src.Spec.Credentials.SecretRef.Namespace,
			UsernameKey: src.Spec.Credentials.SecretRef.UsernameKey,
			PasswordKey: src.Spec.Credentials.SecretRef.PasswordKey,
		}
	}
	return nil
}

// PostgreSQLServerConfig //

func (in *PostgreSQLServerConfig) SetupWebhookWithManager(mgr ctrl.Manager, validator webhook.CustomValidator) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).WithValidator(validator).
		Complete()
}

func (in *PostgreSQLServerConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v2alpha1.PostgreSQLServerConfig)
	dst.ObjectMeta = in.ObjectMeta
	dst.Spec.Address = in.Spec.Address
	dst.Spec.Credentials.Username = in.Spec.Credentials.Username
	dst.Spec.Credentials.Password = in.Spec.Credentials.Password
	if in.Spec.Credentials.SecretRef != nil {
		dst.Spec.Credentials.SecretRef = &v2alpha1.DatabaseCredentialsSecretRef{
			Name:        in.Spec.Credentials.SecretRef.Name,
			Namespace:   in.Spec.Credentials.SecretRef.Namespace,
			UsernameKey: in.Spec.Credentials.SecretRef.UsernameKey,
			PasswordKey: in.Spec.Credentials.SecretRef.PasswordKey,
		}

	}
	return nil
}

func (in *PostgreSQLServerConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2alpha1.PostgreSQLServerConfig)
	in.ObjectMeta = src.ObjectMeta
	in.Spec.Address = src.Spec.Address
	in.Spec.Credentials.Username = src.Spec.Credentials.Username
	in.Spec.Credentials.Password = src.Spec.Credentials.Password
	if src.Spec.Credentials.SecretRef != nil {
		in.Spec.Credentials.SecretRef = &DatabaseCredentialsSecretRef{
			Name:        src.Spec.Credentials.SecretRef.Name,
			Namespace:   src.Spec.Credentials.SecretRef.Namespace,
			UsernameKey: src.Spec.Credentials.SecretRef.UsernameKey,
			PasswordKey: src.Spec.Credentials.SecretRef.PasswordKey,
		}
	}
	return nil
}

// KafkaServerConfig //

func (ksc *KafkaServerConfig) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(ksc).
		Complete()
}

func (ksc *KafkaServerConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v2alpha1.KafkaServerConfig)
	dst.ObjectMeta = ksc.ObjectMeta
	// convert each spec attribute
	dst.Spec.Service.Name = ksc.Spec.Service.Name
	dst.Spec.Service.Kind = ksc.Spec.Service.Kind
	dst.Spec.NoAutoCreateIntentsForOperator = ksc.Spec.NoAutoCreateIntentsForOperator
	dst.Spec.Addr = ksc.Spec.Addr
	dst.Spec.TLS = v2alpha1.TLSSource{
		CertFile:   ksc.Spec.TLS.CertFile,
		KeyFile:    ksc.Spec.TLS.KeyFile,
		RootCAFile: ksc.Spec.TLS.RootCAFile,
	}
	dst.Spec.Topics = make([]v2alpha1.TopicConfig, len(ksc.Spec.Topics))
	for i, topic := range ksc.Spec.Topics {
		dst.Spec.Topics[i].Topic = topic.Topic
		dst.Spec.Topics[i].Pattern = v2alpha1.ResourcePatternType(topic.Pattern)
		dst.Spec.Topics[i].ClientIdentityRequired = topic.ClientIdentityRequired
		dst.Spec.Topics[i].IntentsRequired = topic.IntentsRequired
	}
	return nil
}

func (ksc *KafkaServerConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2alpha1.KafkaServerConfig)
	ksc.ObjectMeta = src.ObjectMeta
	// convert each spec attribute
	ksc.Spec.Service.Name = src.Spec.Service.Name
	ksc.Spec.Service.Kind = src.Spec.Service.Kind
	ksc.Spec.NoAutoCreateIntentsForOperator = src.Spec.NoAutoCreateIntentsForOperator
	ksc.Spec.Addr = src.Spec.Addr
	ksc.Spec.TLS = TLSSource{
		CertFile:   src.Spec.TLS.CertFile,
		KeyFile:    src.Spec.TLS.KeyFile,
		RootCAFile: src.Spec.TLS.RootCAFile,
	}
	ksc.Spec.Topics = make([]TopicConfig, len(src.Spec.Topics))
	for i, topic := range src.Spec.Topics {
		ksc.Spec.Topics[i].Topic = topic.Topic
		ksc.Spec.Topics[i].Pattern = ResourcePatternType(topic.Pattern)
		ksc.Spec.Topics[i].ClientIdentityRequired = topic.ClientIdentityRequired
		ksc.Spec.Topics[i].IntentsRequired = topic.IntentsRequired
	}
	return nil

}

// ProtectedService //

func (in *ProtectedService) SetupWebhookWithManager(mgr ctrl.Manager, validator webhook.CustomValidator) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).WithValidator(validator).
		Complete()
}

func (in *ProtectedService) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v2alpha1.ProtectedService)
	dst.ObjectMeta = in.ObjectMeta
	dst.Spec.Name = in.Spec.Name
	dst.Spec.Kind = in.Spec.Kind
	return nil
}

func (in *ProtectedService) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2alpha1.ProtectedService)
	in.ObjectMeta = src.ObjectMeta
	in.Spec.Name = src.Spec.Name
	in.Spec.Kind = src.Spec.Kind
	return nil
}

// ClientIntents //

func (in *ClientIntents) SetupWebhookWithManager(mgr ctrl.Manager, validator webhook.CustomValidator) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).WithValidator(validator).
		Complete()
}

func (in *ClientIntents) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v2alpha1.ClientIntents)
	dst.ObjectMeta = in.ObjectMeta
	dst.Status.UpToDate = in.Status.UpToDate
	dst.Status.ObservedGeneration = in.Status.ObservedGeneration
	dst.Status.ResolvedIPs = lo.Map(in.Status.ResolvedIPs, func(resolvedIPs ResolvedIPs, _ int) v2alpha1.ResolvedIPs {
		return v2alpha1.ResolvedIPs{DNS: resolvedIPs.DNS, IPs: slices.Clone(resolvedIPs.IPs)}
	})
	if dst.Spec == nil {
		dst.Spec = &v2alpha1.IntentsSpec{}
	}
	dst.Spec.Workload.Name = in.Spec.Service.Name
	dst.Spec.Workload.Kind = in.Spec.Service.Kind
	dst.Spec.Targets = make([]v2alpha1.Target, len(in.Spec.Calls))
	for i, call := range in.Spec.Calls {
		if call.IsTargetInCluster() && call.Type != IntentTypeKafka {
			var name string
			if call.Name == call.GetTargetServerName() {
				name = call.Name
			} else {
				name = call.GetServerFullyQualifiedName(in.Namespace)
			}
			kubernetesTarget := v2alpha1.KubernetesTarget{Name: name, Kind: call.GetTargetServerKind()}
			if kubernetesTarget.Kind == serviceidentity.KindOtterizeLegacy {
				// This is an internal kind the user doesn't need to see it
				kubernetesTarget.Kind = ""
			}
			if call.Type == IntentTypeHTTP && len(call.HTTPResources) > 0 {
				httpTargets := make([]v2alpha1.HTTPTarget, len(call.HTTPResources))
				for j, http := range call.HTTPResources {
					methods := lo.Map(http.Methods, func(method HTTPMethod, _ int) v2alpha1.HTTPMethod { return v2alpha1.HTTPMethod(method) })
					httpTargets[j] = v2alpha1.HTTPTarget{Path: http.Path, Methods: methods}
				}
				kubernetesTarget.HTTP = httpTargets
			}
			dst.Spec.Targets[i] = v2alpha1.Target{Kubernetes: &kubernetesTarget}
			continue
		}
		if call.Type == IntentTypeKafka && len(call.Topics) > 0 {
			topics := lo.Map(call.Topics, func(topic KafkaTopic, _ int) v2alpha1.KafkaTopic {
				return v2alpha1.KafkaTopic{Name: topic.Name, Operations: lo.Map(topic.Operations, func(operation KafkaOperation, _ int) v2alpha1.KafkaOperation {
					return v2alpha1.KafkaOperation(operation)
				})}
			})
			dst.Spec.Targets[i] = v2alpha1.Target{Kafka: lo.ToPtr(v2alpha1.KafkaTarget{Name: call.Name, Topics: topics})}
		}
		if call.Type == IntentTypeDatabase && len(call.DatabaseResources) > 0 {
			tables := lo.Map(call.DatabaseResources, func(resource DatabaseResource, _ int) v2alpha1.SQLPrivileges {
				return v2alpha1.SQLPrivileges{Table: resource.Table, DatabaseName: resource.DatabaseName, Operations: lo.Map(resource.Operations, func(operation DatabaseOperation, _ int) v2alpha1.DatabaseOperation {
					return v2alpha1.DatabaseOperation(operation)
				})}
			})
			dst.Spec.Targets[i] = v2alpha1.Target{SQL: lo.ToPtr(v2alpha1.SQLTarget{Name: call.Name, Privileges: tables})}
			continue
		}
		if call.Type == IntentTypeAWS {
			dst.Spec.Targets[i] = v2alpha1.Target{AWS: lo.ToPtr(v2alpha1.AWSTarget{ARN: call.Name, Actions: call.AWSActions})}
			continue
		}
		if call.Type == IntentTypeGCP {
			dst.Spec.Targets[i] = v2alpha1.Target{GCP: lo.ToPtr(v2alpha1.GCPTarget{Resource: call.Name, Permissions: call.GCPPermissions})}
			continue
		}
		if call.Type == IntentTypeAzure {
			dst.Spec.Targets[i] = v2alpha1.Target{Azure: lo.ToPtr(v2alpha1.AzureTarget{Scope: call.Name, Roles: call.AzureRoles})}
			if call.AzureKeyVaultPolicy == nil {
				continue
			}
			dst.Spec.Targets[i].Azure.KeyVaultPolicy = &v2alpha1.AzureKeyVaultPolicy{}
			dst.Spec.Targets[i].Azure.KeyVaultPolicy.KeyPermissions = lo.Map(call.AzureKeyVaultPolicy.KeyPermissions, func(permission AzureKeyVaultKeyPermission, _ int) v2alpha1.AzureKeyVaultKeyPermission {
				return v2alpha1.AzureKeyVaultKeyPermission(permission)
			})
			dst.Spec.Targets[i].Azure.KeyVaultPolicy.SecretPermissions = lo.Map(call.AzureKeyVaultPolicy.SecretPermissions, func(permission AzureKeyVaultSecretPermission, _ int) v2alpha1.AzureKeyVaultSecretPermission {
				return v2alpha1.AzureKeyVaultSecretPermission(permission)
			})
			dst.Spec.Targets[i].Azure.KeyVaultPolicy.CertificatePermissions = lo.Map(call.AzureKeyVaultPolicy.CertificatePermissions, func(permission AzureKeyVaultCertificatePermission, _ int) v2alpha1.AzureKeyVaultCertificatePermission {
				return v2alpha1.AzureKeyVaultCertificatePermission(permission)
			})
			dst.Spec.Targets[i].Azure.KeyVaultPolicy.StoragePermissions = lo.Map(call.AzureKeyVaultPolicy.StoragePermissions, func(permission AzureKeyVaultStoragePermission, _ int) v2alpha1.AzureKeyVaultStoragePermission {
				return v2alpha1.AzureKeyVaultStoragePermission(permission)
			})
		}
		if call.Type == IntentTypeInternet && call.Internet != nil {
			dst.Spec.Targets[i] = v2alpha1.Target{Internet: lo.ToPtr(v2alpha1.Internet{Domains: call.Internet.Domains, Ports: call.Internet.Ports, Ips: call.Internet.Ips})}
		}
	}
	return nil
}

func (in *ClientIntents) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2alpha1.ClientIntents)
	in.ObjectMeta = src.ObjectMeta
	in.Status.UpToDate = src.Status.UpToDate
	in.Status.ObservedGeneration = src.Status.ObservedGeneration
	in.Status.ResolvedIPs = lo.Map(src.Status.ResolvedIPs, func(resolvedIPs v2alpha1.ResolvedIPs, _ int) ResolvedIPs {
		return ResolvedIPs{DNS: resolvedIPs.DNS, IPs: slices.Clone(resolvedIPs.IPs)}
	})
	in.Spec = &IntentsSpec{}
	in.Spec.Service.Name = src.Spec.Workload.Name
	in.Spec.Service.Kind = src.Spec.Workload.Kind
	in.Spec.Calls = make([]Intent, len(src.Spec.Targets))
	for i, target := range src.Spec.Targets {
		if target.IsTargetTheKubernetesAPIServer(src.Namespace) {
			// Using "svc:kubernetes.default" was a common use case in v1alpha3 -
			// therefore we prefer to convert to this form.
			in.Spec.Calls[i] = Intent{Name: "svc:" + target.GetTargetServerNameAsWritten()}
			continue
		}
		if target.Kubernetes != nil {
			in.Spec.Calls[i] = Intent{Name: target.Kubernetes.Name, Kind: target.Kubernetes.Kind}
			if target.Kubernetes.HTTP != nil {
				in.Spec.Calls[i].HTTPResources = lo.Map(target.Kubernetes.HTTP, func(http v2alpha1.HTTPTarget, _ int) HTTPResource {
					return HTTPResource{Path: http.Path, Methods: lo.Map(http.Methods, func(method v2alpha1.HTTPMethod, _ int) HTTPMethod { return HTTPMethod(method) })}
				})
				in.Spec.Calls[i].Type = IntentTypeHTTP
			}
			continue
		}
		if target.Service != nil {
			in.Spec.Calls[i] = Intent{Name: target.Service.Name, Kind: serviceidentity.KindService}
			if target.Service.HTTP != nil {
				in.Spec.Calls[i].HTTPResources = lo.Map(target.Service.HTTP, func(http v2alpha1.HTTPTarget, _ int) HTTPResource {
					return HTTPResource{Path: http.Path, Methods: lo.Map(http.Methods, func(method v2alpha1.HTTPMethod, _ int) HTTPMethod { return HTTPMethod(method) })}
				})
				in.Spec.Calls[i].Type = IntentTypeHTTP
			}
			continue
		}
		if target.Kafka != nil {
			in.Spec.Calls[i] = Intent{Type: IntentTypeKafka, Name: target.Kafka.Name, Topics: lo.Map(target.Kafka.Topics, func(topic v2alpha1.KafkaTopic, _ int) KafkaTopic {
				return KafkaTopic{Name: topic.Name, Operations: lo.Map(topic.Operations, func(operation v2alpha1.KafkaOperation, _ int) KafkaOperation { return KafkaOperation(operation) })}
			})}
			continue
		}
		if target.SQL != nil {
			in.Spec.Calls[i] = Intent{Type: IntentTypeDatabase, Name: target.SQL.Name, DatabaseResources: lo.Map(target.SQL.Privileges, func(permission v2alpha1.SQLPrivileges, _ int) DatabaseResource {
				return DatabaseResource{Table: permission.Table, DatabaseName: permission.DatabaseName, Operations: lo.Map(permission.Operations, func(operation v2alpha1.DatabaseOperation, _ int) DatabaseOperation {
					return DatabaseOperation(operation)
				})}
			})}
			continue
		}
		if target.AWS != nil {
			in.Spec.Calls[i] = Intent{Type: IntentTypeAWS, Name: target.AWS.ARN, AWSActions: target.AWS.Actions}
			continue
		}
		if target.GCP != nil {
			in.Spec.Calls[i] = Intent{Type: IntentTypeGCP, Name: target.GCP.Resource, GCPPermissions: target.GCP.Permissions}
			continue
		}
		if target.Azure != nil {
			in.Spec.Calls[i] = Intent{Type: IntentTypeAzure, Name: target.Azure.Scope, AzureRoles: target.Azure.Roles}
			if target.Azure.KeyVaultPolicy == nil {
				continue
			}
			in.Spec.Calls[i].AzureKeyVaultPolicy = &AzureKeyVaultPolicy{}
			in.Spec.Calls[i].AzureKeyVaultPolicy.KeyPermissions = lo.Map(target.Azure.KeyVaultPolicy.KeyPermissions, func(permission v2alpha1.AzureKeyVaultKeyPermission, _ int) AzureKeyVaultKeyPermission {
				return AzureKeyVaultKeyPermission(permission)
			})
			in.Spec.Calls[i].AzureKeyVaultPolicy.SecretPermissions = lo.Map(target.Azure.KeyVaultPolicy.SecretPermissions, func(permission v2alpha1.AzureKeyVaultSecretPermission, _ int) AzureKeyVaultSecretPermission {
				return AzureKeyVaultSecretPermission(permission)
			})
			in.Spec.Calls[i].AzureKeyVaultPolicy.CertificatePermissions = lo.Map(target.Azure.KeyVaultPolicy.CertificatePermissions, func(permission v2alpha1.AzureKeyVaultCertificatePermission, _ int) AzureKeyVaultCertificatePermission {
				return AzureKeyVaultCertificatePermission(permission)
			})
			in.Spec.Calls[i].AzureKeyVaultPolicy.StoragePermissions = lo.Map(target.Azure.KeyVaultPolicy.StoragePermissions, func(permission v2alpha1.AzureKeyVaultStoragePermission, _ int) AzureKeyVaultStoragePermission {
				return AzureKeyVaultStoragePermission(permission)
			})
		}
		if target.Internet != nil {
			in.Spec.Calls[i] = Intent{Type: IntentTypeInternet, Internet: &Internet{Domains: target.Internet.Domains, Ports: target.Internet.Ports, Ips: target.Internet.Ips}}
		}
	}
	return nil
}
