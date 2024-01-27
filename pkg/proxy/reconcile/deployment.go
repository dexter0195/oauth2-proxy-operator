package reconcile

import (
	"context"
	"fmt"

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

func Deployment(ctx context.Context, params Params) (ctrl.Result, error) {
	log := params.Log
	oauth2proxy := params.Instance
	desiredDep, err := deploymentForOAuth2Proxy(params)
	if err != nil {
		log.Error(err, "Failed to define new Deployment resource for OAuth2Proxy")
		// The following implementation will update the status
		meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: TypeAvailableOAuth2Proxy,
			Status: metav1.ConditionFalse, Reason: "Reconciling",
			Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", oauth2proxy.Name, err)})
		if err := params.Client.Status().Update(ctx, oauth2proxy); err != nil {
			log.Error(err, "Failed to update OAuth2Proxy status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	currentDep := &appsv1.Deployment{}
	err = params.Client.Get(ctx, types.NamespacedName{Name: oauth2proxy.Name, Namespace: oauth2proxy.Namespace}, currentDep)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		log.Info("Creating a new Deployment",
			"Deployment.Namespace", desiredDep.Namespace, "Deployment.Name", desiredDep.Name)
		if err = params.Client.Create(ctx, desiredDep); err != nil {
			log.Error(err, "Failed to create new Deployment",
				"Deployment.Namespace", desiredDep.Namespace, "Deployment.Name", desiredDep.Name)
			return ctrl.Result{}, err
		}

		// Deployment created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	} else {
		// Deployment already exists, check that was is deployed is what we expect.
		updated := currentDep.DeepCopy()
		if updated.Annotations == nil {
			updated.Annotations = map[string]string{}
		}
		if updated.Labels == nil {
			updated.Labels = map[string]string{}
		}
		updated.Spec = desiredDep.Spec
		updated.ObjectMeta.OwnerReferences = desiredDep.ObjectMeta.OwnerReferences

		for k, v := range desiredDep.ObjectMeta.Annotations {
			updated.ObjectMeta.Annotations[k] = v
		}
		for k, v := range desiredDep.ObjectMeta.Labels {
			updated.ObjectMeta.Labels[k] = v
		}
		patch := client.MergeFrom(currentDep)
		if err := params.Client.Patch(ctx, updated, patch); err != nil {
			log.Error(err, "Failed to patch Deployment")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// deploymentForOAuth2Proxy returns a OAuth2Proxy Deployment object
func deploymentForOAuth2Proxy(params Params) (*appsv1.Deployment, error) {
	oauth2proxy := params.Instance
	ls := proxy.LabelsForOAuth2Proxy(oauth2proxy.Name)
	replicas := oauth2proxy.Spec.Size

	// Get the Operand image
	image, err := proxy.ImageForOAuth2Proxy()
	if err != nil {
		return nil, err
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      oauth2proxy.Name,
			Namespace: oauth2proxy.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						// IMPORTANT: seccomProfile was introduced with Kubernetes 1.19
						// If you are looking for to produce solutions to be supported
						// on lower versions you must remove this option.
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Image:           image,
						Name:            "oauth2proxy",
						ImagePullPolicy: corev1.PullIfNotPresent,
						// Ensure restrictive context for the container
						// More info: https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted
						SecurityContext: &corev1.SecurityContext{
							RunAsNonRoot:             &[]bool{true}[0],
							RunAsUser:                &[]int64{1001}[0],
							AllowPrivilegeEscalation: &[]bool{false}[0],
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{
									"ALL",
								},
							},
						},
						//Command: []string{"/bin/sleep", fmt.Sprintf("1000")},
						Command: []string{"/bin/oauth2-proxy", "--config=/etc/oauth2-proxy.cfg"},
						EnvFrom: []corev1.EnvFromSource{
							{
								SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: oauth2proxy.Spec.EnvFromExistingSecret.SecretRef["name"]},
								},
							},
						},
						// Add the volumeMount to the container
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "config",
								MountPath: "/etc/oauth2-proxy.cfg",
								SubPath:   "config.cfg",
								ReadOnly:  true,
							},
						},
					}},
					// add a volume of the config map to the container
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: oauth2proxy.Name},
								},
							},
						},
					},
				},
			},
		},
	}

	// Set the ownerRef for the Deployment
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
	if err := ctrl.SetControllerReference(oauth2proxy, dep, params.Scheme); err != nil {
		return nil, err
	}
	return dep, nil
}
