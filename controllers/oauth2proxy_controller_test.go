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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	oauth2proxyv1alpha1 "github.com/dexter0195/oauth2-proxy-operator/api/v1alpha1"
	oauth2proxyreconcile "github.com/dexter0195/oauth2-proxy-operator/pkg/proxy/reconcile"
)

var _ = Describe("OAuth2Proxy controller", func() {
	Context("OAuth2Proxy controller test", func() {

		const OAuth2ProxyName = "test-oauth2proxy"

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      OAuth2ProxyName,
				Namespace: OAuth2ProxyName,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: OAuth2ProxyName, Namespace: OAuth2ProxyName}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))

			By("Setting the Image ENV VAR which stores the Operand image")
			err = os.Setenv("OAUTH2PROXY_IMAGE", "example.com/image:test")
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)

			By("Removing the Image ENV VAR which stores the Operand image")
			_ = os.Unsetenv("OAUTH2PROXY_IMAGE")
		})

		It("should successfully reconcile a custom resource for OAuth2Proxy", func() {
			By("Creating the custom resource for the Kind OAuth2Proxy")
			oauth2proxy := &oauth2proxyv1alpha1.OAuth2Proxy{}
			err := k8sClient.Get(ctx, typeNamespaceName, oauth2proxy)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				oauth2proxy := &oauth2proxyv1alpha1.OAuth2Proxy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      OAuth2ProxyName,
						Namespace: namespace.Name,
					},
					Spec: oauth2proxyv1alpha1.OAuth2ProxySpec{
						Size: 1,
					},
				}

				err = k8sClient.Create(ctx, oauth2proxy)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &oauth2proxyv1alpha1.OAuth2Proxy{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Reconciling the custom resource created")
			oauth2proxyReconciler := &OAuth2ProxyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = oauth2proxyReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(Not(HaveOccurred()))

			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func() error {
				found := &appsv1.Deployment{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Checking the latest Status Condition added to the OAuth2Proxy instance")
			Eventually(func() error {
				if oauth2proxy.Status.Conditions != nil && len(oauth2proxy.Status.Conditions) != 0 {
					latestStatusCondition := oauth2proxy.Status.Conditions[len(oauth2proxy.Status.Conditions)-1]
					expectedLatestStatusCondition := metav1.Condition{Type: oauth2proxyreconcile.TypeAvailableOAuth2Proxy,
						Status: metav1.ConditionTrue, Reason: "Reconciling",
						Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", oauth2proxy.Name, oauth2proxy.Spec.Size)}
					if latestStatusCondition != expectedLatestStatusCondition {
						return fmt.Errorf("The latest status condition added to the oauth2proxy instance is not as expected")
					}
				}
				return nil
			}, time.Minute, time.Second).Should(Succeed())
		})
	})
})
