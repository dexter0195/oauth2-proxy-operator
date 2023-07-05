package reconcile

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/dexter0195/oauth2-proxy-operator/pkg/proxy"
)

func ConfigMap(ctx context.Context, params Params) error {

	// check if configmap exists, if not create a new one
	foundCM := &corev1.ConfigMap{}
	oauth2proxy := params.Instance
	err := params.Client.Get(ctx, types.NamespacedName{Name: oauth2proxy.Name, Namespace: oauth2proxy.Namespace}, foundCM)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new configmap
		cm, err := configMapForOAuth2Proxy(params)
		if err != nil {
			params.Log.Error(err, "Failed to define new ConfigMap resource for OAuth2Proxy")

			// The following implementation will update the status
			meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: TypeAvailableOAuth2Proxy,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create ConfigMap for the custom resource (%s): (%s)", oauth2proxy.Name, err)})

			if err := params.Client.Status().Update(ctx, oauth2proxy); err != nil {
				params.Log.Error(err, "Failed to update OAuth2Proxy status")
				return err
			}
		}
		params.Log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", cm.Namespace, "ConfigMap.Name", cm.Name)
		if err := params.Client.Create(ctx, cm); err != nil {
			params.Log.Error(err, "Failed to create new ConfigMap", "ConfigMap.Namespace", cm.Namespace, "ConfigMap.Name", cm.Name)
			return err
		}
	}
	return nil
}

func configMapForOAuth2Proxy(params Params) (*corev1.ConfigMap, error) {
	oauth2proxy := params.Instance
	ls := proxy.LabelsForOAuth2Proxy(oauth2proxy.Name)

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      oauth2proxy.Name,
			Namespace: oauth2proxy.Namespace,
			Labels:    ls,
		},
		// add the data from the oauth2proxy spec field config to the configmap data
		Data: map[string]string{
			"config.cfg": oauth2proxy.Spec.Config,
		},
	}
	// Set the ownerRef for the ConfigMap
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(oauth2proxy, cm, params.Scheme); err != nil {
		return nil, err
	}

	return cm, nil
}
