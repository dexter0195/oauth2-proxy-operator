package reconcile

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oauth2proxyv1alpha1 "github.com/dexter0195/oauth2-proxy-operator/api/v1alpha1"
)

// Params holds the reconciliation-specific parameters.
type Params struct {
	Client   client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Request  ctrl.Request
	Instance *oauth2proxyv1alpha1.OAuth2Proxy
}
