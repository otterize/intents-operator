package access_annotation

import (
	"encoding/json"
	"fmt"
	otterizev1alpha3 "github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/errors"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
	"github.com/otterize/nilable"
	v1 "k8s.io/api/core/v1"
	"strings"
	"unicode"
)

type CalledByClient struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func ParseAdditionalAccess(pod *v1.Pod) ([]nilable.Nilable[serviceidentity.ServiceIdentity], bool, error) {
	annotation := pod.GetAnnotations()
	if annotation == nil {
		return nil, false, nil
	}
	value, ok := annotation[otterizev1alpha3.OtterizePodCalledByAnnotationKey]
	if !ok {
		return nil, false, nil
	}
	clients, err := parseAnnotation(value)
	if err != nil {
		return nil, false, errors.Wrap(err)
	}
	if len(clients) == 0 {
		return nil, false, nil
	}
	return clients, true, nil
}

func parseAnnotation(value string) ([]nilable.Nilable[serviceidentity.ServiceIdentity], error) {
	var annotationIntents []nilable.Nilable[CalledByClient]
	err := json.Unmarshal([]byte(value), &annotationIntents)
	if err != nil {
		return nil, errors.Errorf("failed to parse access annotation: %s", err)
	}

	identities := make([]nilable.Nilable[serviceidentity.ServiceIdentity], 0)
	for _, intent := range annotationIntents {
		if !intent.Set {
			identities = append(identities, nilable.Nilable[serviceidentity.ServiceIdentity]{})
		}
		name, namespace, kind, err := parseName(intent.Item.Name, intent.Item.Kind)
		if err != nil {
			return nil, errors.Wrap(err)
		}
		identity := serviceidentity.ServiceIdentity{
			Name:      name,
			Namespace: namespace,
			Kind:      kind,
		}
		identities = append(identities, nilable.From(identity))
	}
	return identities, nil
}

func parseName(name string, kind string) (string, string, string, error) {
	parts := strings.Split(name, ".")
	if len(parts) != 2 {
		return "", "", "", errors.Wrap(fmt.Errorf("bad called-by annotation client name '%s' should be in the format 'name.namespace'", name))
	}
	name = parts[0]
	namespace := parts[1]

	if len(kind) == 0 {
		return name, namespace, "", errors.Wrap(fmt.Errorf("kind is required"))
	}
	if !unicode.IsUpper(rune(kind[0])) {
		return name, namespace, "", errors.Wrap(fmt.Errorf("kind '%s' should start with a capital letter", kind))
	}
	return name, namespace, kind, nil
}
