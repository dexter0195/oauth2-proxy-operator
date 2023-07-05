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

	"github.com/dexter0195/oauth2-proxy-operator/pkg/proxy"
)

func Deployment(ctx context.Context, params Params) (ctrl.Result, error) {
	foundDep := &appsv1.Deployment{}
	oauth2proxy := params.Instance
	log := params.Log
	err := params.Client.Get(ctx, types.NamespacedName{Name: oauth2proxy.Name, Namespace: oauth2proxy.Namespace}, foundDep)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		dep, err := deploymentForOAuth2Proxy(params)
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

		log.Info("Creating a new Deployment",
			"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		if err = params.Client.Create(ctx, dep); err != nil {
			log.Error(err, "Failed to create new Deployment",
				"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
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
	}

	// Ensure the deployment size is the same as the spec
	if err = checkReplicas(ctx, params, foundDep); err != nil {
		return ctrl.Result{}, err
	} else {
		return ctrl.Result{Requeue: true}, nil
	}
}

func checkReplicas(ctx context.Context, params Params, deployment *appsv1.Deployment) error {

	// The CRD API is defining that the OAuth2Proxy type, have a OAuth2ProxySpec.Size field
	// to set the quantity of Deployment instances is the desired state on the cluster.
	// Therefore, the following code will ensure the Deployment size is the same as defined
	// via the Size spec of the Custom Resource which we are reconciling.
	oauth2proxy := params.Instance
	log := params.Log
	size := oauth2proxy.Spec.Size
	if *deployment.Spec.Replicas != size {
		deployment.Spec.Replicas = &size
		if err := params.Client.Update(ctx, deployment); err != nil {
			log.Error(err, "Failed to update Deployment",
				"Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)

			// Re-fetch the oauth2proxy Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := params.Client.Get(ctx, params.Request.NamespacedName, oauth2proxy); err != nil {
				log.Error(err, "Failed to re-fetch oauth2proxy")
				return err
			}

			// The following implementation will update the status
			meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: TypeAvailableOAuth2Proxy,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", oauth2proxy.Name, err)})

			if err := params.Client.Status().Update(ctx, oauth2proxy); err != nil {
				log.Error(err, "Failed to update OAuth2Proxy status")
				return err
			}

			return err
		}

		return nil
	}

	// The following implementation will update the status
	meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: TypeAvailableOAuth2Proxy,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", oauth2proxy.Name, size)})

	if err := params.Client.Status().Update(ctx, oauth2proxy); err != nil {
		log.Error(err, "Failed to update OAuth2Proxy status")
		return err
	}
	return nil
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
