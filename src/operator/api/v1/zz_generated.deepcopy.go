//go:build !ignore_autogenerated

/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AzureKeyVaultPolicy) DeepCopyInto(out *AzureKeyVaultPolicy) {
	*out = *in
	if in.CertificatePermissions != nil {
		in, out := &in.CertificatePermissions, &out.CertificatePermissions
		*out = make([]AzureKeyVaultCertificatePermission, len(*in))
		copy(*out, *in)
	}
	if in.KeyPermissions != nil {
		in, out := &in.KeyPermissions, &out.KeyPermissions
		*out = make([]AzureKeyVaultKeyPermission, len(*in))
		copy(*out, *in)
	}
	if in.SecretPermissions != nil {
		in, out := &in.SecretPermissions, &out.SecretPermissions
		*out = make([]AzureKeyVaultSecretPermission, len(*in))
		copy(*out, *in)
	}
	if in.StoragePermissions != nil {
		in, out := &in.StoragePermissions, &out.StoragePermissions
		*out = make([]AzureKeyVaultStoragePermission, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AzureKeyVaultPolicy.
func (in *AzureKeyVaultPolicy) DeepCopy() *AzureKeyVaultPolicy {
	if in == nil {
		return nil
	}
	out := new(AzureKeyVaultPolicy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientIntents) DeepCopyInto(out *ClientIntents) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec != nil {
		in, out := &in.Spec, &out.Spec
		*out = new(IntentsSpec)
		(*in).DeepCopyInto(*out)
	}
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientIntents.
func (in *ClientIntents) DeepCopy() *ClientIntents {
	if in == nil {
		return nil
	}
	out := new(ClientIntents)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClientIntents) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientIntentsList) DeepCopyInto(out *ClientIntentsList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClientIntents, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientIntentsList.
func (in *ClientIntentsList) DeepCopy() *ClientIntentsList {
	if in == nil {
		return nil
	}
	out := new(ClientIntentsList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClientIntentsList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseCredentials) DeepCopyInto(out *DatabaseCredentials) {
	*out = *in
	if in.SecretRef != nil {
		in, out := &in.SecretRef, &out.SecretRef
		*out = new(DatabaseCredentialsSecretRef)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseCredentials.
func (in *DatabaseCredentials) DeepCopy() *DatabaseCredentials {
	if in == nil {
		return nil
	}
	out := new(DatabaseCredentials)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseCredentialsSecretRef) DeepCopyInto(out *DatabaseCredentialsSecretRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseCredentialsSecretRef.
func (in *DatabaseCredentialsSecretRef) DeepCopy() *DatabaseCredentialsSecretRef {
	if in == nil {
		return nil
	}
	out := new(DatabaseCredentialsSecretRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseResource) DeepCopyInto(out *DatabaseResource) {
	*out = *in
	if in.Operations != nil {
		in, out := &in.Operations, &out.Operations
		*out = make([]DatabaseOperation, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseResource.
func (in *DatabaseResource) DeepCopy() *DatabaseResource {
	if in == nil {
		return nil
	}
	out := new(DatabaseResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HTTPResource) DeepCopyInto(out *HTTPResource) {
	*out = *in
	if in.Methods != nil {
		in, out := &in.Methods, &out.Methods
		*out = make([]HTTPMethod, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HTTPResource.
func (in *HTTPResource) DeepCopy() *HTTPResource {
	if in == nil {
		return nil
	}
	out := new(HTTPResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Intent) DeepCopyInto(out *Intent) {
	*out = *in
	if in.Topics != nil {
		in, out := &in.Topics, &out.Topics
		*out = make([]KafkaTopic, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.HTTPResources != nil {
		in, out := &in.HTTPResources, &out.HTTPResources
		*out = make([]HTTPResource, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.DatabaseResources != nil {
		in, out := &in.DatabaseResources, &out.DatabaseResources
		*out = make([]DatabaseResource, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.AWSActions != nil {
		in, out := &in.AWSActions, &out.AWSActions
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.GCPPermissions != nil {
		in, out := &in.GCPPermissions, &out.GCPPermissions
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.AzureRoles != nil {
		in, out := &in.AzureRoles, &out.AzureRoles
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.AzureKeyVaultPolicy != nil {
		in, out := &in.AzureKeyVaultPolicy, &out.AzureKeyVaultPolicy
		*out = new(AzureKeyVaultPolicy)
		(*in).DeepCopyInto(*out)
	}
	if in.Internet != nil {
		in, out := &in.Internet, &out.Internet
		*out = new(Internet)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Intent.
func (in *Intent) DeepCopy() *Intent {
	if in == nil {
		return nil
	}
	out := new(Intent)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IntentsSpec) DeepCopyInto(out *IntentsSpec) {
	*out = *in
	out.Service = in.Service
	if in.Calls != nil {
		in, out := &in.Calls, &out.Calls
		*out = make([]Intent, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IntentsSpec.
func (in *IntentsSpec) DeepCopy() *IntentsSpec {
	if in == nil {
		return nil
	}
	out := new(IntentsSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IntentsStatus) DeepCopyInto(out *IntentsStatus) {
	*out = *in
	if in.ResolvedIPs != nil {
		in, out := &in.ResolvedIPs, &out.ResolvedIPs
		*out = make([]ResolvedIPs, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new IntentsStatus.
func (in *IntentsStatus) DeepCopy() *IntentsStatus {
	if in == nil {
		return nil
	}
	out := new(IntentsStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Internet) DeepCopyInto(out *Internet) {
	*out = *in
	if in.Domains != nil {
		in, out := &in.Domains, &out.Domains
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Ips != nil {
		in, out := &in.Ips, &out.Ips
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Ports != nil {
		in, out := &in.Ports, &out.Ports
		*out = make([]int, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Internet.
func (in *Internet) DeepCopy() *Internet {
	if in == nil {
		return nil
	}
	out := new(Internet)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KafkaServerConfig) DeepCopyInto(out *KafkaServerConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KafkaServerConfig.
func (in *KafkaServerConfig) DeepCopy() *KafkaServerConfig {
	if in == nil {
		return nil
	}
	out := new(KafkaServerConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *KafkaServerConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KafkaServerConfigList) DeepCopyInto(out *KafkaServerConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]KafkaServerConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KafkaServerConfigList.
func (in *KafkaServerConfigList) DeepCopy() *KafkaServerConfigList {
	if in == nil {
		return nil
	}
	out := new(KafkaServerConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *KafkaServerConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KafkaServerConfigSpec) DeepCopyInto(out *KafkaServerConfigSpec) {
	*out = *in
	out.Service = in.Service
	out.TLS = in.TLS
	if in.Topics != nil {
		in, out := &in.Topics, &out.Topics
		*out = make([]TopicConfig, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KafkaServerConfigSpec.
func (in *KafkaServerConfigSpec) DeepCopy() *KafkaServerConfigSpec {
	if in == nil {
		return nil
	}
	out := new(KafkaServerConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KafkaServerConfigStatus) DeepCopyInto(out *KafkaServerConfigStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KafkaServerConfigStatus.
func (in *KafkaServerConfigStatus) DeepCopy() *KafkaServerConfigStatus {
	if in == nil {
		return nil
	}
	out := new(KafkaServerConfigStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KafkaTopic) DeepCopyInto(out *KafkaTopic) {
	*out = *in
	if in.Operations != nil {
		in, out := &in.Operations, &out.Operations
		*out = make([]KafkaOperation, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KafkaTopic.
func (in *KafkaTopic) DeepCopy() *KafkaTopic {
	if in == nil {
		return nil
	}
	out := new(KafkaTopic)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MySQLServerConfig) DeepCopyInto(out *MySQLServerConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MySQLServerConfig.
func (in *MySQLServerConfig) DeepCopy() *MySQLServerConfig {
	if in == nil {
		return nil
	}
	out := new(MySQLServerConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MySQLServerConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MySQLServerConfigList) DeepCopyInto(out *MySQLServerConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]MySQLServerConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MySQLServerConfigList.
func (in *MySQLServerConfigList) DeepCopy() *MySQLServerConfigList {
	if in == nil {
		return nil
	}
	out := new(MySQLServerConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MySQLServerConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MySQLServerConfigSpec) DeepCopyInto(out *MySQLServerConfigSpec) {
	*out = *in
	in.Credentials.DeepCopyInto(&out.Credentials)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MySQLServerConfigSpec.
func (in *MySQLServerConfigSpec) DeepCopy() *MySQLServerConfigSpec {
	if in == nil {
		return nil
	}
	out := new(MySQLServerConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MySQLServerConfigStatus) DeepCopyInto(out *MySQLServerConfigStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MySQLServerConfigStatus.
func (in *MySQLServerConfigStatus) DeepCopy() *MySQLServerConfigStatus {
	if in == nil {
		return nil
	}
	out := new(MySQLServerConfigStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLServerConfig) DeepCopyInto(out *PostgreSQLServerConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLServerConfig.
func (in *PostgreSQLServerConfig) DeepCopy() *PostgreSQLServerConfig {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLServerConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PostgreSQLServerConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLServerConfigList) DeepCopyInto(out *PostgreSQLServerConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PostgreSQLServerConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLServerConfigList.
func (in *PostgreSQLServerConfigList) DeepCopy() *PostgreSQLServerConfigList {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLServerConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PostgreSQLServerConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLServerConfigSpec) DeepCopyInto(out *PostgreSQLServerConfigSpec) {
	*out = *in
	in.Credentials.DeepCopyInto(&out.Credentials)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLServerConfigSpec.
func (in *PostgreSQLServerConfigSpec) DeepCopy() *PostgreSQLServerConfigSpec {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLServerConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLServerConfigStatus) DeepCopyInto(out *PostgreSQLServerConfigStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLServerConfigStatus.
func (in *PostgreSQLServerConfigStatus) DeepCopy() *PostgreSQLServerConfigStatus {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLServerConfigStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProtectedService) DeepCopyInto(out *ProtectedService) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProtectedService.
func (in *ProtectedService) DeepCopy() *ProtectedService {
	if in == nil {
		return nil
	}
	out := new(ProtectedService)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProtectedService) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProtectedServiceList) DeepCopyInto(out *ProtectedServiceList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ProtectedService, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProtectedServiceList.
func (in *ProtectedServiceList) DeepCopy() *ProtectedServiceList {
	if in == nil {
		return nil
	}
	out := new(ProtectedServiceList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ProtectedServiceList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProtectedServiceSpec) DeepCopyInto(out *ProtectedServiceSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProtectedServiceSpec.
func (in *ProtectedServiceSpec) DeepCopy() *ProtectedServiceSpec {
	if in == nil {
		return nil
	}
	out := new(ProtectedServiceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProtectedServiceStatus) DeepCopyInto(out *ProtectedServiceStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProtectedServiceStatus.
func (in *ProtectedServiceStatus) DeepCopy() *ProtectedServiceStatus {
	if in == nil {
		return nil
	}
	out := new(ProtectedServiceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResolvedIPs) DeepCopyInto(out *ResolvedIPs) {
	*out = *in
	if in.IPs != nil {
		in, out := &in.IPs, &out.IPs
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResolvedIPs.
func (in *ResolvedIPs) DeepCopy() *ResolvedIPs {
	if in == nil {
		return nil
	}
	out := new(ResolvedIPs)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Service) DeepCopyInto(out *Service) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Service.
func (in *Service) DeepCopy() *Service {
	if in == nil {
		return nil
	}
	out := new(Service)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSSource) DeepCopyInto(out *TLSSource) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSSource.
func (in *TLSSource) DeepCopy() *TLSSource {
	if in == nil {
		return nil
	}
	out := new(TLSSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TopicConfig) DeepCopyInto(out *TopicConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TopicConfig.
func (in *TopicConfig) DeepCopy() *TopicConfig {
	if in == nil {
		return nil
	}
	out := new(TopicConfig)
	in.DeepCopyInto(out)
	return out
}
