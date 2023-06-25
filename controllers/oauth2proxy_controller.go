/*
Copyright 2023.

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

package controllers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	oauth2proxyv1alpha1 "github.com/dexter0195/oauth2-proxy-operator/api/v1alpha1"
)

const oauth2proxyFinalizer = "oauth2proxy.oauth2proxy-operator.dexter0195.com/finalizer"

// Definitions to manage status conditions
const (
	// typeAvailableOAuth2Proxy represents the status of the Deployment reconciliation
	typeAvailableOAuth2Proxy = "Available"
	// typeDegradedOAuth2Proxy represents the status used when the custom resource is deleted and the finalizer operations are must to occur.
	typeDegradedOAuth2Proxy = "Degraded"
)

// OAuth2ProxyReconciler reconciles a OAuth2Proxy object
type OAuth2ProxyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// The following markers are used to generate the rules permissions (RBAC) on config/rbac using controller-gen
// when the command <make manifests> is executed.
// To know more about markers see: https://book.kubebuilder.io/reference/markers.html

//+kubebuilder:rbac:groups=oauth2proxy.oauth2proxy-operator.dexter0195.com,resources=oauth2proxies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=oauth2proxy.oauth2proxy-operator.dexter0195.com,resources=oauth2proxies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=oauth2proxy.oauth2proxy-operator.dexter0195.com,resources=oauth2proxies/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.

// It is essential for the controller's reconciliation loop to be idempotent. By following the Operator
// pattern you will create Controllers which provide a reconcile function
// responsible for synchronizing resources until the desired state is reached on the cluster.
// Breaking this recommendation goes against the design principles of controller-runtime.
// and may lead to unforeseen consequences such as resources becoming stuck and requiring manual intervention.
// For further info:
// - About Operator Pattern: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
// - About Controllers: https://kubernetes.io/docs/concepts/architecture/controller/
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *OAuth2ProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the OAuth2Proxy instance
	// The purpose is check if the Custom Resource for the Kind OAuth2Proxy
	// is applied on the cluster if not we return nil to stop the reconciliation
	oauth2proxy := &oauth2proxyv1alpha1.OAuth2Proxy{}
	err := r.Get(ctx, req.NamespacedName, oauth2proxy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			log.Info("oauth2proxy resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get oauth2proxy")
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	if oauth2proxy.Status.Conditions == nil || len(oauth2proxy.Status.Conditions) == 0 {
		meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: typeAvailableOAuth2Proxy, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, oauth2proxy); err != nil {
			log.Error(err, "Failed to update OAuth2Proxy status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the oauth2proxy Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, oauth2proxy); err != nil {
			log.Error(err, "Failed to re-fetch oauth2proxy")
			return ctrl.Result{}, err
		}
	}

	// Let's add a finalizer. Then, we can define some operations which should
	// occurs before the custom resource to be deleted.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers
	if !controllerutil.ContainsFinalizer(oauth2proxy, oauth2proxyFinalizer) {
		log.Info("Adding Finalizer for OAuth2Proxy")
		if ok := controllerutil.AddFinalizer(oauth2proxy, oauth2proxyFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, oauth2proxy); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Check if the OAuth2Proxy instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isOAuth2ProxyMarkedToBeDeleted := oauth2proxy.GetDeletionTimestamp() != nil
	if isOAuth2ProxyMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(oauth2proxy, oauth2proxyFinalizer) {
			log.Info("Performing Finalizer Operations for OAuth2Proxy before delete CR")

			// Let's add here an status "Downgrade" to define that this resource begin its process to be terminated.
			meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: typeDegradedOAuth2Proxy,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", oauth2proxy.Name)})

			if err := r.Status().Update(ctx, oauth2proxy); err != nil {
				log.Error(err, "Failed to update OAuth2Proxy status")
				return ctrl.Result{}, err
			}

			// Perform all operations required before remove the finalizer and allow
			// the Kubernetes API to remove the custom resource.
			r.doFinalizerOperationsForOAuth2Proxy(oauth2proxy)

			// TODO(user): If you add operations to the doFinalizerOperationsForOAuth2Proxy method
			// then you need to ensure that all worked fine before deleting and updating the Downgrade status
			// otherwise, you should requeue here.

			// Re-fetch the oauth2proxy Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, oauth2proxy); err != nil {
				log.Error(err, "Failed to re-fetch oauth2proxy")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: typeDegradedOAuth2Proxy,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", oauth2proxy.Name)})

			if err := r.Status().Update(ctx, oauth2proxy); err != nil {
				log.Error(err, "Failed to update OAuth2Proxy status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for OAuth2Proxy after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(oauth2proxy, oauth2proxyFinalizer); !ok {
				log.Error(err, "Failed to remove finalizer for OAuth2Proxy")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, oauth2proxy); err != nil {
				log.Error(err, "Failed to remove finalizer for OAuth2Proxy")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// check if configmap exists, if not create a new one
	foundCM := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: oauth2proxy.Name, Namespace: oauth2proxy.Namespace}, foundCM)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new configmap
		cm, err := r.configMapForOAuth2Proxy(oauth2proxy)
		if err != nil {
			log.Error(err, "Failed to define new ConfigMap resource for OAuth2Proxy")

			// The following implementation will update the status
			meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: typeAvailableOAuth2Proxy,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create ConfigMap for the custom resource (%s): (%s)", oauth2proxy.Name, err)})

			if err := r.Status().Update(ctx, oauth2proxy); err != nil {
				log.Error(err, "Failed to update OAuth2Proxy status")
				return ctrl.Result{}, err
			}
		}
		log.Info("Creating a new ConfigMap", "ConfigMap.Namespace", cm.Namespace, "ConfigMap.Name", cm.Name)
		if err := r.Create(ctx, cm); err != nil {
			log.Error(err, "Failed to create new ConfigMap", "ConfigMap.Namespace", cm.Namespace, "ConfigMap.Name", cm.Name)
			return ctrl.Result{}, err
		}
	}

	// Check if the deployment already exists, if not create a new one
	foundDep := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: oauth2proxy.Name, Namespace: oauth2proxy.Namespace}, foundDep)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new deployment
		dep, err := r.deploymentForOAuth2Proxy(oauth2proxy)
		if err != nil {
			log.Error(err, "Failed to define new Deployment resource for OAuth2Proxy")

			// The following implementation will update the status
			meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: typeAvailableOAuth2Proxy,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", oauth2proxy.Name, err)})

			if err := r.Status().Update(ctx, oauth2proxy); err != nil {
				log.Error(err, "Failed to update OAuth2Proxy status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		log.Info("Creating a new Deployment",
			"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		if err = r.Create(ctx, dep); err != nil {
			log.Error(err, "Failed to create new Deployment",
				"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		// Deployment created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	// The CRD API is defining that the OAuth2Proxy type, have a OAuth2ProxySpec.Size field
	// to set the quantity of Deployment instances is the desired state on the cluster.
	// Therefore, the following code will ensure the Deployment size is the same as defined
	// via the Size spec of the Custom Resource which we are reconciling.
	size := oauth2proxy.Spec.Size
	if *foundDep.Spec.Replicas != size {
		foundDep.Spec.Replicas = &size
		if err = r.Update(ctx, foundDep); err != nil {
			log.Error(err, "Failed to update Deployment",
				"Deployment.Namespace", foundDep.Namespace, "Deployment.Name", foundDep.Name)

			// Re-fetch the oauth2proxy Custom Resource before update the status
			// so that we have the latest state of the resource on the cluster and we will avoid
			// raise the issue "the object has been modified, please apply
			// your changes to the latest version and try again" which would re-trigger the reconciliation
			if err := r.Get(ctx, req.NamespacedName, oauth2proxy); err != nil {
				log.Error(err, "Failed to re-fetch oauth2proxy")
				return ctrl.Result{}, err
			}

			// The following implementation will update the status
			meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: typeAvailableOAuth2Proxy,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", oauth2proxy.Name, err)})

			if err := r.Status().Update(ctx, oauth2proxy); err != nil {
				log.Error(err, "Failed to update OAuth2Proxy status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		// Now, that we update the size we want to requeue the reconciliation
		// so that we can ensure that we have the latest state of the resource before
		// update. Also, it will help ensure the desired state on the cluster
		return ctrl.Result{Requeue: true}, nil
	}

	// The following implementation will update the status
	meta.SetStatusCondition(&oauth2proxy.Status.Conditions, metav1.Condition{Type: typeAvailableOAuth2Proxy,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", oauth2proxy.Name, size)})

	if err := r.Status().Update(ctx, oauth2proxy); err != nil {
		log.Error(err, "Failed to update OAuth2Proxy status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// finalizeOAuth2Proxy will perform the required operations before delete the CR.
func (r *OAuth2ProxyReconciler) doFinalizerOperationsForOAuth2Proxy(cr *oauth2proxyv1alpha1.OAuth2Proxy) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// Note: It is not recommended to use finalizers with the purpose of delete resources which are
	// created and managed in the reconciliation. These ones, such as the Deployment created on this reconcile,
	// are defined as depended of the custom resource. See that we use the method ctrl.SetControllerReference.
	// to set the ownerRef which means that the Deployment will be deleted by the Kubernetes API.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

func (r *OAuth2ProxyReconciler) configMapForOAuth2Proxy(
	oauth2proxy *oauth2proxyv1alpha1.OAuth2Proxy) (*corev1.ConfigMap, error) {
	ls := labelsForOAuth2Proxy(oauth2proxy.Name)

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
	if err := ctrl.SetControllerReference(oauth2proxy, cm, r.Scheme); err != nil {
		return nil, err
	}

	return cm, nil
}

// deploymentForOAuth2Proxy returns a OAuth2Proxy Deployment object
func (r *OAuth2ProxyReconciler) deploymentForOAuth2Proxy(
	oauth2proxy *oauth2proxyv1alpha1.OAuth2Proxy) (*appsv1.Deployment, error) {
	ls := labelsForOAuth2Proxy(oauth2proxy.Name)
	replicas := oauth2proxy.Spec.Size

	// Get the Operand image
	image, err := imageForOAuth2Proxy()
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
						Command: []string{"/bin/oauth2-proxy", fmt.Sprintf("--config=/etc/oauth2-proxy.cfg")},
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
	if err := ctrl.SetControllerReference(oauth2proxy, dep, r.Scheme); err != nil {
		return nil, err
	}
	return dep, nil
}

// labelsForOAuth2Proxy returns the labels for selecting the resources
// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func labelsForOAuth2Proxy(name string) map[string]string {
	var imageTag string
	image, err := imageForOAuth2Proxy()
	if err == nil {
		imageTag = strings.Split(image, ":")[1]
	}
	return map[string]string{"app.kubernetes.io/name": "OAuth2Proxy",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/part-of":    "oauth2-proxy-operator",
		"app.kubernetes.io/created-by": "controller-manager",
	}
}

// imageForOAuth2Proxy gets the Operand image which is managed by this controller
// from the OAUTH2PROXY_IMAGE environment variable defined in the config/manager/manager.yaml
func imageForOAuth2Proxy() (string, error) {
	var imageEnvVar = "OAUTH2PROXY_IMAGE"
	image, found := os.LookupEnv(imageEnvVar)
	if !found {
		return "", fmt.Errorf("Unable to find %s environment variable with the image", imageEnvVar)
	}
	return image, nil
}

// SetupWithManager sets up the controller with the Manager.
// Note that the Deployment will be also watched in order to ensure its
// desirable state on the cluster
func (r *OAuth2ProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oauth2proxyv1alpha1.OAuth2Proxy{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
