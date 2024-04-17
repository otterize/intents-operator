package serviceidentity

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/samber/lo"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

const (
	MaxOtterizeNameLength = 20
	MaxNamespaceLength    = 20
)

type ServiceIdentity struct {
	Name      string
	Namespace string
	Kind      string
	// OwnerObject used to resolve the service name. May be nil if service name was resolved using annotation.
	OwnerObject client.Object
}

const KindService = "Service"
const KindOtterizeLegacy = "OttrLegacy"

func (si *ServiceIdentity) GetFormattedOtterizeIdentity() string {
	if si.Kind == KindService {
		return GetFormattedOtterizeIdentityWithKind(si.Name, si.Namespace, si.Kind)
	}
	return GetFormattedOtterizeIdentity(si.Name, si.Namespace)
}

func (si *ServiceIdentity) GetFormattedOtterizeIdentityWithKind() string {
	if si.Kind == "" || si.Kind == KindOtterizeLegacy {
		return GetFormattedOtterizeIdentity(si.Name, si.Namespace)
	}
	return GetFormattedOtterizeIdentityWithKind(si.Name, si.Namespace, si.Kind)
}

func (si *ServiceIdentity) GetFormattedOtterizeIdentityWithoutKind() string {
	return GetFormattedOtterizeIdentity(si.Name, si.Namespace)
}

func (si *ServiceIdentity) GetName() string {
	return si.Name
}

func (si *ServiceIdentity) GetNameWithKind() string {
	return lo.Ternary(si.Kind == "" || si.Kind == KindOtterizeLegacy, si.Name, fmt.Sprintf("%s-%s", si.Name, strings.ToLower(si.Kind)))
}

func (si *ServiceIdentity) String() string {
	return fmt.Sprintf("%s/%s/%s", si.Kind, si.Namespace, si.Name)
}

// GetFormattedOtterizeIdentity truncates names and namespaces to a 20 char len string (if required)
// It also adds a short md5 hash of the full name+ns string and returns the formatted string
// This is due to Kubernetes' limit on 63 char label keys/values
func GetFormattedOtterizeIdentity(name, ns string) string {
	// Get MD5 for full length "name-namespace" string
	hash := md5.Sum([]byte(fmt.Sprintf("%s-%s", name, ns)))

	// Truncate name and namespace to 20 chars each
	if len(name) > MaxOtterizeNameLength {
		name = name[:MaxOtterizeNameLength]
	}

	if len(ns) > MaxNamespaceLength {
		ns = ns[:MaxNamespaceLength]
	}
	// A 6 char hash, even though truncated, leaves 2 ^ 48 combinations which should be enough
	// for unique identities in a k8s cluster
	hashSuffix := hex.EncodeToString(hash[:])[:6]

	return fmt.Sprintf("%s-%s-%s", name, ns, hashSuffix)

}

func GetFormattedOtterizeIdentityWithKind(name, ns, kind string) string {
	// Get MD5 for full length "name-namespace" string
	hash := md5.Sum([]byte(fmt.Sprintf("%s-%s-%s", name, ns, kind)))

	// Truncate name and namespace to 20 chars each
	if len(name) > MaxOtterizeNameLength {
		name = name[:MaxOtterizeNameLength]
	}

	if len(ns) > MaxNamespaceLength {
		ns = ns[:MaxNamespaceLength]
	}

	if len(kind) > 5 {
		kind = kind[:5]
	}
	// A 6 char hash, even though truncated, leaves 2 ^ 48 combinations which should be enough
	// for unique identities in a k8s cluster
	hashSuffix := hex.EncodeToString(hash[:])[:6]

	return fmt.Sprintf("%s-%s-%s-%s", name, ns, strings.ToLower(kind), hashSuffix)

}

func NewFromPodOwner(podOwner client.Object) *ServiceIdentity {
	return &ServiceIdentity{
		Name:      podOwner.GetName(),
		Namespace: podOwner.GetNamespace(),
		Kind:      podOwner.GetObjectKind().GroupVersionKind().Kind,
	}
}
