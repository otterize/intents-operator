package v2alpha1

import (
	"github.com/otterize/intents-operator/src/operator/api/v2beta1"
	"github.com/otterize/intents-operator/src/shared/errors"
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

// convert to and convert from functions for MySQLServerConfig (hub version is v2beta1)
func (in *MySQLServerConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v2beta1.MySQLServerConfig)
	dst.ObjectMeta = in.ObjectMeta
	dst.Spec.Address = in.Spec.Address
	dst.Spec.Credentials.Username = in.Spec.Credentials.Username
	dst.Spec.Credentials.Password = in.Spec.Credentials.Password
	if in.Spec.Credentials.SecretRef != nil {
		dst.Spec.Credentials.SecretRef = &v2beta1.DatabaseCredentialsSecretRef{
			Name:        in.Spec.Credentials.SecretRef.Name,
			Namespace:   in.Spec.Credentials.SecretRef.Namespace,
			UsernameKey: in.Spec.Credentials.SecretRef.UsernameKey,
			PasswordKey: in.Spec.Credentials.SecretRef.PasswordKey,
		}
	}
	return nil
}

func (in *MySQLServerConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2beta1.MySQLServerConfig)
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
	dst := dstRaw.(*v2beta1.PostgreSQLServerConfig)
	dst.ObjectMeta = in.ObjectMeta
	dst.Spec.Address = in.Spec.Address
	dst.Spec.Credentials.Username = in.Spec.Credentials.Username
	dst.Spec.Credentials.Password = in.Spec.Credentials.Password
	if in.Spec.Credentials.SecretRef != nil {
		dst.Spec.Credentials.SecretRef = &v2beta1.DatabaseCredentialsSecretRef{
			Name:        in.Spec.Credentials.SecretRef.Name,
			Namespace:   in.Spec.Credentials.SecretRef.Namespace,
			UsernameKey: in.Spec.Credentials.SecretRef.UsernameKey,
			PasswordKey: in.Spec.Credentials.SecretRef.PasswordKey,
		}
	}
	return nil
}

func (in *PostgreSQLServerConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2beta1.PostgreSQLServerConfig)
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
	dst := dstRaw.(*v2beta1.KafkaServerConfig)
	dst.ObjectMeta = ksc.ObjectMeta
	dst.Spec.Addr = ksc.Spec.Addr
	dst.Spec.TLS.CertFile = ksc.Spec.TLS.CertFile
	dst.Spec.TLS.KeyFile = ksc.Spec.TLS.KeyFile
	dst.Spec.TLS.RootCAFile = ksc.Spec.TLS.RootCAFile
	dst.Spec.Workload.Name = ksc.Spec.Workload.Name
	dst.Spec.Workload.Kind = ksc.Spec.Workload.Kind
	dst.Spec.Topics = make([]v2beta1.TopicConfig, len(ksc.Spec.Topics))
	for i, topic := range ksc.Spec.Topics {
		dst.Spec.Topics[i].Topic = topic.Topic
		dst.Spec.Topics[i].Pattern = v2beta1.ResourcePatternType(topic.Pattern)
		dst.Spec.Topics[i].ClientIdentityRequired = topic.ClientIdentityRequired
		dst.Spec.Topics[i].IntentsRequired = topic.IntentsRequired
	}
	dst.Spec.NoAutoCreateIntentsForOperator = ksc.Spec.NoAutoCreateIntentsForOperator
	return nil
}

func (ksc *KafkaServerConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2beta1.KafkaServerConfig)
	ksc.ObjectMeta = src.ObjectMeta
	ksc.Spec.Addr = src.Spec.Addr
	ksc.Spec.TLS.CertFile = src.Spec.TLS.CertFile
	ksc.Spec.TLS.KeyFile = src.Spec.TLS.KeyFile
	ksc.Spec.TLS.RootCAFile = src.Spec.TLS.RootCAFile
	ksc.Spec.Workload.Name = src.Spec.Workload.Name
	ksc.Spec.Workload.Kind = src.Spec.Workload.Kind
	ksc.Spec.Topics = make([]TopicConfig, len(src.Spec.Topics))
	for i, topic := range src.Spec.Topics {
		ksc.Spec.Topics[i].Topic = topic.Topic
		ksc.Spec.Topics[i].Pattern = ResourcePatternType(topic.Pattern)
		ksc.Spec.Topics[i].ClientIdentityRequired = topic.ClientIdentityRequired
		ksc.Spec.Topics[i].IntentsRequired = topic.IntentsRequired
	}
	ksc.Spec.NoAutoCreateIntentsForOperator = src.Spec.NoAutoCreateIntentsForOperator
	return nil
}

// ProtectedService //

func (in *ProtectedService) SetupWebhookWithManager(mgr ctrl.Manager, validator webhook.CustomValidator) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).WithValidator(validator).
		Complete()
}

func (in *ProtectedService) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v2beta1.ProtectedService)
	dst.ObjectMeta = in.ObjectMeta
	dst.Spec.Name = in.Spec.Name
	dst.Spec.Kind = in.Spec.Kind
	return nil
}

func (in *ProtectedService) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2beta1.ProtectedService)
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
	dst := dstRaw.(*v2beta1.ClientIntents)
	dst.ObjectMeta = in.ObjectMeta

	// convert status
	dst.Status.UpToDate = in.Status.UpToDate
	dst.Status.ObservedGeneration = in.Status.ObservedGeneration
	dst.Status.ResolvedIPs = lo.Map(in.Status.ResolvedIPs, func(resolvedIPs ResolvedIPs, _ int) v2beta1.ResolvedIPs {
		return v2beta1.ResolvedIPs{DNS: resolvedIPs.DNS, IPs: slices.Clone(resolvedIPs.IPs)}
	})
	dst.Status.ReviewStatus = v2beta1.ReviewStatus(in.Status.ReviewStatus)

	if dst.Spec == nil {
		dst.Spec = &v2beta1.IntentsSpec{}
	}

	// convert workload
	dst.Spec.Workload.Name = in.Spec.Workload.Name
	dst.Spec.Workload.Kind = in.Spec.Workload.Kind

	// copy targets
	dst.Spec.Targets = make([]v2beta1.Target, len(in.Spec.Targets))
	for i, target := range in.Spec.Targets {
		dst.Spec.Targets[i] = v2beta1.Target{}
		if in.Spec.Targets[i].Service != nil {
			dst.Spec.Targets[i].Service = &v2beta1.ServiceTarget{Name: target.Service.Name}
			if target.Service.HTTP != nil {
				dst.Spec.Targets[i].Service.HTTP = lo.Map(
					target.Service.HTTP,
					func(http HTTPTarget, _ int) v2beta1.HTTPTarget {
						return v2beta1.HTTPTarget{
							Path: http.Path,
							Methods: lo.Map(
								http.Methods,
								func(method HTTPMethod, _ int) v2beta1.HTTPMethod {
									return v2beta1.HTTPMethod(method)
								}),
						}
					})
			}
		}
		if in.Spec.Targets[i].Kubernetes != nil {
			dst.Spec.Targets[i].Kubernetes = &v2beta1.KubernetesTarget{
				Kind: target.Kubernetes.Kind,
				Name: target.Kubernetes.Name,
			}
			if target.Kubernetes.HTTP != nil {
				dst.Spec.Targets[i].Kubernetes.HTTP = lo.Map(
					target.Kubernetes.HTTP,
					func(http HTTPTarget, _ int) v2beta1.HTTPTarget {
						return v2beta1.HTTPTarget{
							Path: http.Path,
							Methods: lo.Map(
								http.Methods,
								func(method HTTPMethod, _ int) v2beta1.HTTPMethod {
									return v2beta1.HTTPMethod(method)
								}),
						}
					})
			}
		}
		if in.Spec.Targets[i].Kafka != nil {
			dst.Spec.Targets[i].Kafka = &v2beta1.KafkaTarget{
				Name: target.Kafka.Name,
				Topics: lo.Map(
					target.Kafka.Topics,
					func(topic KafkaTopic, _ int) v2beta1.KafkaTopic {
						return v2beta1.KafkaTopic{
							Name: topic.Name,
							Operations: lo.Map(
								topic.Operations,
								func(operation KafkaOperation, _ int) v2beta1.KafkaOperation {
									return v2beta1.KafkaOperation(operation)
								}),
						}
					},
				),
			}
		}
		if in.Spec.Targets[i].SQL != nil {
			dst.Spec.Targets[i].SQL = &v2beta1.SQLTarget{
				Name: target.SQL.Name,
				Privileges: lo.Map(
					target.SQL.Privileges,
					func(privilege SQLPrivileges, _ int) v2beta1.SQLPrivileges {
						return v2beta1.SQLPrivileges{
							DatabaseName: privilege.DatabaseName,
							Table:        privilege.Table,
							Operations: lo.Map(
								privilege.Operations,
								func(operation DatabaseOperation, _ int) v2beta1.DatabaseOperation {
									return v2beta1.DatabaseOperation(operation)
								},
							),
						}
					},
				),
			}
		}
		if in.Spec.Targets[i].GCP != nil {
			dst.Spec.Targets[i].GCP = &v2beta1.GCPTarget{
				Resource:    target.GCP.Resource,
				Permissions: slices.Clone(target.GCP.Permissions),
			}
		}
		if in.Spec.Targets[i].AWS != nil {
			dst.Spec.Targets[i].AWS = &v2beta1.AWSTarget{
				ARN:     target.AWS.ARN,
				Actions: slices.Clone(target.AWS.Actions),
			}
		}
		if in.Spec.Targets[i].Azure != nil {
			dst.Spec.Targets[i].Azure = &v2beta1.AzureTarget{Scope: target.Azure.Scope}
			if target.Azure.Actions != nil {
				dst.Spec.Targets[i].Azure.Actions = lo.Map(
					target.Azure.Actions,
					func(action AzureAction, _ int) v2beta1.AzureAction {
						return v2beta1.AzureAction(action)
					},
				)
			}
			if target.Azure.DataActions != nil {
				dst.Spec.Targets[i].Azure.DataActions = lo.Map(
					target.Azure.DataActions,
					func(dataAction AzureDataAction, _ int) v2beta1.AzureDataAction {
						return v2beta1.AzureDataAction(dataAction)
					},
				)
			}
			dst.Spec.Targets[i].Azure.Roles = slices.Clone(target.Azure.Roles)
			if target.Azure.KeyVaultPolicy != nil {
				dst.Spec.Targets[i].Azure.KeyVaultPolicy = &v2beta1.AzureKeyVaultPolicy{
					CertificatePermissions: lo.Map(
						target.Azure.KeyVaultPolicy.CertificatePermissions,
						func(permission AzureKeyVaultCertificatePermission, _ int) v2beta1.AzureKeyVaultCertificatePermission {
							return v2beta1.AzureKeyVaultCertificatePermission(permission)
						},
					),
					KeyPermissions: lo.Map(
						target.Azure.KeyVaultPolicy.KeyPermissions,
						func(permission AzureKeyVaultKeyPermission, _ int) v2beta1.AzureKeyVaultKeyPermission {
							return v2beta1.AzureKeyVaultKeyPermission(permission)
						},
					),
					SecretPermissions: lo.Map(
						target.Azure.KeyVaultPolicy.SecretPermissions,
						func(permission AzureKeyVaultSecretPermission, _ int) v2beta1.AzureKeyVaultSecretPermission {
							return v2beta1.AzureKeyVaultSecretPermission(permission)
						},
					),
					StoragePermissions: lo.Map(
						target.Azure.KeyVaultPolicy.StoragePermissions,
						func(permission AzureKeyVaultStoragePermission, _ int) v2beta1.AzureKeyVaultStoragePermission {
							return v2beta1.AzureKeyVaultStoragePermission(permission)
						},
					),
				}
			}
		}
		if target.Internet != nil {
			dst.Spec.Targets[i].Internet = &v2beta1.Internet{
				Domains: slices.Clone(target.Internet.Domains),
				Ips:     slices.Clone(target.Internet.Ips),
				Ports:   slices.Clone(target.Internet.Ports),
			}
		}
	}
	return nil
}

func (in *ClientIntents) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2beta1.ClientIntents)
	in.ObjectMeta = src.ObjectMeta

	// convert status
	in.Status.UpToDate = src.Status.UpToDate
	in.Status.ObservedGeneration = src.Status.ObservedGeneration
	in.Status.ResolvedIPs = lo.Map(src.Status.ResolvedIPs, func(resolvedIPs v2beta1.ResolvedIPs, _ int) ResolvedIPs {
		return ResolvedIPs{DNS: resolvedIPs.DNS, IPs: slices.Clone(resolvedIPs.IPs)}
	})
	in.Status.ReviewStatus = ReviewStatus(src.Status.ReviewStatus)

	if in.Spec == nil {
		in.Spec = &IntentsSpec{}
	}

	// convert workload
	in.Spec.Workload.Name = src.Spec.Workload.Name
	in.Spec.Workload.Kind = src.Spec.Workload.Kind

	// copy targets
	in.Spec.Targets = make([]Target, len(src.Spec.Targets))
	for i, target := range src.Spec.Targets {
		in.Spec.Targets[i] = Target{}
		if src.Spec.Targets[i].Service != nil {
			in.Spec.Targets[i].Service = &ServiceTarget{Name: target.Service.Name}
			if target.Service.HTTP != nil {
				in.Spec.Targets[i].Service.HTTP = lo.Map(
					target.Service.HTTP,
					func(http v2beta1.HTTPTarget, _ int) HTTPTarget {
						return HTTPTarget{
							Path: http.Path,
							Methods: lo.Map(
								http.Methods,
								func(method v2beta1.HTTPMethod, _ int) HTTPMethod {
									return HTTPMethod(method)
								},
							),
						}
					},
				)
			}
		}
		if src.Spec.Targets[i].Kubernetes != nil {
			in.Spec.Targets[i].Kubernetes = &KubernetesTarget{
				Kind: target.Kubernetes.Kind,
				Name: target.Kubernetes.Name,
			}
			if target.Kubernetes.HTTP != nil {
				in.Spec.Targets[i].Kubernetes.HTTP = lo.Map(
					target.Kubernetes.HTTP,
					func(http v2beta1.HTTPTarget, _ int) HTTPTarget {
						return HTTPTarget{
							Path: http.Path,
							Methods: lo.Map(
								http.Methods,
								func(method v2beta1.HTTPMethod, _ int) HTTPMethod {
									return HTTPMethod(method)
								},
							),
						}
					},
				)
			}
		}
		if src.Spec.Targets[i].Kafka != nil {
			in.Spec.Targets[i].Kafka = &KafkaTarget{
				Name: target.Kafka.Name,
				Topics: lo.Map(
					target.Kafka.Topics,
					func(topic v2beta1.KafkaTopic, _ int) KafkaTopic {
						return KafkaTopic{
							Name: topic.Name,
							Operations: lo.Map(
								topic.Operations,
								func(operation v2beta1.KafkaOperation, _ int) KafkaOperation {
									return KafkaOperation(operation)
								},
							),
						}
					},
				),
			}
		}
		if src.Spec.Targets[i].SQL != nil {
			in.Spec.Targets[i].SQL = &SQLTarget{
				Name: target.SQL.Name,
				Privileges: lo.Map(
					target.SQL.Privileges,
					func(privilege v2beta1.SQLPrivileges, _ int) SQLPrivileges {
						return SQLPrivileges{
							DatabaseName: privilege.DatabaseName,
							Table:        privilege.Table,
							Operations: lo.Map(
								privilege.Operations,
								func(operation v2beta1.DatabaseOperation, _ int) DatabaseOperation {
									return DatabaseOperation(operation)
								},
							),
						}
					},
				),
			}
		}
		if src.Spec.Targets[i].GCP != nil {
			in.Spec.Targets[i].GCP = &GCPTarget{
				Resource:    target.GCP.Resource,
				Permissions: slices.Clone(target.GCP.Permissions),
			}
		}
		if src.Spec.Targets[i].AWS != nil {
			in.Spec.Targets[i].AWS = &AWSTarget{
				ARN:     target.AWS.ARN,
				Actions: slices.Clone(target.AWS.Actions),
			}
		}
		if src.Spec.Targets[i].Azure != nil {
			in.Spec.Targets[i].Azure = &AzureTarget{Scope: target.Azure.Scope}
			if target.Azure.Actions != nil {
				in.Spec.Targets[i].Azure.Actions = lo.Map(
					target.Azure.Actions,
					func(action v2beta1.AzureAction, _ int) AzureAction {
						return AzureAction(action)
					},
				)
			}
			if target.Azure.DataActions != nil {
				in.Spec.Targets[i].Azure.DataActions = lo.Map(
					target.Azure.DataActions,
					func(dataAction v2beta1.AzureDataAction, _ int) AzureDataAction {
						return AzureDataAction(dataAction)
					},
				)
			}
			in.Spec.Targets[i].Azure.Roles = slices.Clone(target.Azure.Roles)
			if target.Azure.KeyVaultPolicy != nil {
				in.Spec.Targets[i].Azure.KeyVaultPolicy = &AzureKeyVaultPolicy{
					CertificatePermissions: lo.Map(
						target.Azure.KeyVaultPolicy.CertificatePermissions,
						func(permission v2beta1.AzureKeyVaultCertificatePermission, _ int) AzureKeyVaultCertificatePermission {
							return AzureKeyVaultCertificatePermission(permission)
						},
					),
					KeyPermissions: lo.Map(
						target.Azure.KeyVaultPolicy.KeyPermissions,
						func(permission v2beta1.AzureKeyVaultKeyPermission, _ int) AzureKeyVaultKeyPermission {
							return AzureKeyVaultKeyPermission(permission)
						},
					),
					SecretPermissions: lo.Map(
						target.Azure.KeyVaultPolicy.SecretPermissions,
						func(permission v2beta1.AzureKeyVaultSecretPermission, _ int) AzureKeyVaultSecretPermission {
							return AzureKeyVaultSecretPermission(permission)
						},
					),
					StoragePermissions: lo.Map(
						target.Azure.KeyVaultPolicy.StoragePermissions,
						func(permission v2beta1.AzureKeyVaultStoragePermission, _ int) AzureKeyVaultStoragePermission {
							return AzureKeyVaultStoragePermission(permission)
						},
					),
				}
			}
		}
		if target.Internet != nil {
			in.Spec.Targets[i].Internet = &Internet{
				Domains: slices.Clone(target.Internet.Domains),
				Ips:     slices.Clone(target.Internet.Ips),
				Ports:   slices.Clone(target.Internet.Ports),
			}
		}
	}
	return nil
}

// ApprovedClientIntents //

func (in *ApprovedClientIntents) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).Complete()
}

func (in *ApprovedClientIntents) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v2beta1.ApprovedClientIntents)
	dst.ObjectMeta = in.ObjectMeta

	srcClientIntents := &ClientIntents{}
	srcClientIntents.ObjectMeta = in.ObjectMeta
	srcClientIntents.Spec = in.Spec

	dst.Spec = &v2beta1.IntentsSpec{}
	dstClientIntents := &v2beta1.ClientIntents{}
	dstClientIntents.ObjectMeta = in.ObjectMeta

	err := srcClientIntents.ConvertTo(dstClientIntents)
	if err != nil {
		return errors.Wrap(err)
	}
	dst.Spec = dstClientIntents.Spec

	dst.Status.UpToDate = in.Status.UpToDate
	dst.Status.ObservedGeneration = in.Status.ObservedGeneration
	dst.Status.ResolvedIPs = lo.Map(in.Status.ResolvedIPs, func(resolvedIPs ResolvedIPs, _ int) v2beta1.ResolvedIPs {
		return v2beta1.ResolvedIPs{DNS: resolvedIPs.DNS, IPs: slices.Clone(resolvedIPs.IPs)}
	})
	return nil
}

func (in *ApprovedClientIntents) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v2beta1.ApprovedClientIntents)
	srcClientIntents := &v2beta1.ClientIntents{}
	srcClientIntents.ObjectMeta = src.ObjectMeta
	srcClientIntents.Spec = src.Spec

	dstClientIntents := &ClientIntents{}
	err := dstClientIntents.ConvertFrom(srcClientIntents)
	if err != nil {
		return errors.Wrap(err)
	}

	in.ObjectMeta = src.ObjectMeta
	in.Spec = dstClientIntents.Spec
	in.Status.UpToDate = src.Status.UpToDate
	in.Status.ObservedGeneration = src.Status.ObservedGeneration
	in.Status.ResolvedIPs = lo.Map(src.Status.ResolvedIPs, func(resolvedIPs v2beta1.ResolvedIPs, _ int) ResolvedIPs {
		return ResolvedIPs{DNS: resolvedIPs.DNS, IPs: slices.Clone(resolvedIPs.IPs)}
	})
	return nil
}
