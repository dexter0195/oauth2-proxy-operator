package reconcile

import (
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dexter0195/oauth2-proxy-operator/pkg/proxy"
)

func ConfigMap(ctx context.Context, params Params) error {

	// Create the desirec ConfigMap object
	desiredCM, err := desiredConfigMap(params)
	if err != nil {
		params.Log.Error(err, "Failed to define new ConfigMap resource for OAuth2Proxy")
		return err
	}
	// check if configmap exists, if not create a new one
	currentCM := &corev1.ConfigMap{}
	oauth2proxy := params.Instance
	err = params.Client.Get(ctx, types.NamespacedName{Name: oauth2proxy.Name, Namespace: oauth2proxy.Namespace}, currentCM)
	if err != nil && apierrors.IsNotFound(err) {
		// The following implementation will update the status
		meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: TypeAvailableOAuth2Proxy,
			Status: metav1.ConditionFalse, Reason: "Reconciling",
			Message: fmt.Sprintf("Failed to create ConfigMap for the custom resource (%s): (%s)", oauth2proxy.Name, err)})

		if err := params.Client.Status().Update(ctx, oauth2proxy); err != nil {
			params.Log.Error(err, "Failed to update OAuth2Proxy status")
			return err
		}
		params.Log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", desiredCM.Namespace, "ConfigMap.Name", desiredCM.Name)
		if err := params.Client.Create(ctx, desiredCM); err != nil {
			params.Log.Error(err, "Failed to create new ConfigMap", "ConfigMap.Namespace", desiredCM.Namespace, "ConfigMap.Name", desiredCM.Name)
			return err
		}
	} else {
		// check if the desired configmap is different from the current configmap and merge the changes
		updated := currentCM.DeepCopy()
		if updated.Annotations == nil {
			updated.Annotations = map[string]string{}
		}
		if updated.Labels == nil {
			updated.Labels = map[string]string{}
		}

		updated.Data = desiredCM.Data
		updated.BinaryData = desiredCM.BinaryData
		updated.ObjectMeta.OwnerReferences = desiredCM.ObjectMeta.OwnerReferences

		for k, v := range desiredCM.ObjectMeta.Annotations {
			updated.ObjectMeta.Annotations[k] = v
		}
		for k, v := range desiredCM.ObjectMeta.Labels {
			updated.ObjectMeta.Labels[k] = v
		}

		patch := client.MergeFrom(currentCM)

		if configMapChanged(desiredCM, currentCM) {
			params.Recorder.Event(updated, "Normal", "ConfigUpdate ", fmt.Sprintf("OpenTelemetry Config changed - %s/%s", desiredCM.Namespace, desiredCM.Name))
			meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: TypeAvailableOAuth2Proxy,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Updating Configmap and Deployment after change for the custom resource (%s)", oauth2proxy.Name)})

			if err := params.Client.Get(ctx, params.Request.NamespacedName, oauth2proxy); err != nil {
				params.Log.Error(err, "Failed to re-fetch oauth2proxy")
				return err
			}
			if err := params.Client.Patch(ctx, updated, patch); err != nil {
				return fmt.Errorf("failed to apply changes: %w", err)
			}
			// Update the Deployment to deploy new pods with the new configmap
			params.Log.V(2).Info("applied", "configmap.name", desiredCM.Name, "configmap.namespace", desiredCM.Namespace)
			if err := updateDeployment(ctx, params, updated); err != nil {
				return fmt.Errorf("failed to restart deployment: %w", err)
			}
		}

	}
	return nil
}

func updateDeployment(ctx context.Context, params Params, cm *corev1.ConfigMap) error {

	dep := &appsv1.Deployment{}
	oauth2proxy := params.Instance
	// need to check the flow here, if the configmap was not changed then we should not set the status to reconciling
	if err := params.Client.Status().Update(ctx, oauth2proxy); err != nil {
		params.Log.Error(err, "Failed to update OAuth2Proxy status")
		return err
	}
	err := params.Client.Get(ctx, types.NamespacedName{Name: oauth2proxy.Name, Namespace: oauth2proxy.Namespace}, dep)
	if err != nil || apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get deployment while updating the configmap: %w", err)
	}
	// hash the cm object in base64 and set it as the annotation of the dep
	sha := fmt.Sprintf("%x", sha256.Sum256([]byte(cm.Data["config.cfg"])))
	if dep.Spec.Template.ObjectMeta.Annotations == nil {
		dep.Spec.Template.ObjectMeta.Annotations = map[string]string{}
	}
	dep.Spec.Template.ObjectMeta.Annotations["configmap-hash"] = sha
	if err := params.Client.Update(ctx, dep); err != nil {
		return fmt.Errorf("failed to update deployment while updating the configmap: %w", err)
	}

	return nil
}

func desiredConfigMap(params Params) (*corev1.ConfigMap, error) {
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

func configMapChanged(desired *corev1.ConfigMap, actual *corev1.ConfigMap) bool {
	return !reflect.DeepEqual(desired.Data, actual.Data)
}
