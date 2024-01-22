package effectivepolicy

import (
	"github.com/otterize/intents-operator/src/operator/api/v1alpha3"
	"github.com/otterize/intents-operator/src/shared/injectablerecorder"
	"github.com/otterize/intents-operator/src/shared/serviceidresolver/serviceidentity"
)

type ClientCall struct {
	Service             serviceidentity.ServiceIdentity
	IntendedCall        v1alpha3.Intent
	ObjectEventRecorder *injectablerecorder.ObjectEventRecorder
}

type ServiceEffectivePolicy struct {
	Service      serviceidentity.ServiceIdentity
	CalledBy     []ClientCall
	ClientIntent *v1alpha3.ClientIntents
}
