package k8sconf

import (
	"k8s.io/client-go/rest"
	"net/http"
	"net/url"
	ctrl "sigs.k8s.io/controller-runtime"
)

func KubernetesConfigOrDie() *rest.Config {
	conf := ctrl.GetConfigOrDie()
	conf.Proxy = func(*http.Request) (*url.URL, error) {
		// Never use proxy for k8s API
		return nil, nil // nolint
	}
	return conf
}
