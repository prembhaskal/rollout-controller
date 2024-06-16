/*
Copyright 2024.

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

package rollout_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	// "sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/api/v1alpha1"
	"github.com/prembhaskal/rollout-controller/pkg/config"
	rollout "github.com/prembhaskal/rollout-controller/pkg/rollout"
)

var _ = Describe("Flipper Controller", Ordered, func() {
	const defaultns = "default"
	const waitTime = 15 * time.Second
	const pollTime = 3 * time.Second

	matchCriteria := &config.MatchCriteria{}
	ctx := context.Background()

	BeforeAll(func() {
		cm, err := manager.New(cfg, manager.Options{})
		Expect(err).NotTo(HaveOccurred())

		rolloutController := rollout.New(k8sClient, matchCriteria)
		err = rolloutController.SetupWithManager(cm)
		Expect(err).NotTo(HaveOccurred())

		By("Starting the Manager")
		ctx, cancel := context.WithCancel(context.Background())
		// defer cancel()
		DeferCleanup(cancel)
		go func() {
			defer GinkgoRecover()
			Expect(cm.Start(ctx)).NotTo(HaveOccurred())
		}()
	})

	It("Restart a Deployment which is never restarted Before", func() {
		const resourceName = "test-resource"
		const deploymentName = "deployment-one"
		const namespace = "default"
		deployment := createDeployment(deploymentName, namespace, "", map[string]string{"mesh": "true"})

		err := k8sClient.Create(ctx, deployment)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func(g Gomega) error {
			nsName := types.NamespacedName{
				Namespace: namespace,
				Name:      deploymentName,
			}
			var updatedDepObj appsv1.Deployment
			err := k8sClient.Get(ctx, nsName, &updatedDepObj)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(updatedDepObj.Spec.Template.Annotations).NotTo(BeNil())
			restartedAt := updatedDepObj.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"]
			GinkgoWriter.Println("[test log] restartedAt", restartedAt)
			g.Expect(restartedAt).ToNot(BeEmpty())

			_, err = time.Parse(time.RFC3339, restartedAt)
			Expect(err).ToNot(HaveOccurred())

			return nil
		}).WithTimeout(waitTime).WithPolling(pollTime).Should(Succeed())

		err = k8sClient.Delete(ctx, deployment)
		Expect(err).ToNot(HaveOccurred())
	})

	It("does not restart a deployment which does not match default criteria", func() {
		const resourceName = "test-resource"
		const deploymentName = "deployment-two"
		const namespace = "default"
		deployment := createDeployment(deploymentName, namespace, "", map[string]string{"mesh": "false"})

		err := k8sClient.Create(ctx, deployment)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func(g Gomega) error {
			nsName := types.NamespacedName{
				Namespace: namespace,
				Name:      deploymentName,
			}
			var updatedDepObj appsv1.Deployment
			err := k8sClient.Get(ctx, nsName, &updatedDepObj)
			g.Expect(err).ToNot(HaveOccurred())

			if updatedDepObj.Spec.Template.Annotations == nil {
				return nil
			}
			restartedAt := updatedDepObj.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"]
			g.Expect(restartedAt).To(BeEmpty())
			return nil
		}).WithTimeout(waitTime).WithPolling(pollTime).Should(Succeed())

		err = k8sClient.Delete(ctx, deployment)
		Expect(err).ToNot(HaveOccurred())
	})

	It("does not restart a deployment which was already restarted in past interval", func() {
		const resourceName = "test-resource"
		const deploymentName = "deployment-three"
		const namespace = "default"

		pastTime := time.Now().Add(-1 * time.Minute).Format(time.RFC3339)
		GinkgoWriter.Println("[test log] currTime", pastTime)
		deployment := createDeployment(deploymentName, namespace, pastTime, map[string]string{"mesh": "true"})

		err := k8sClient.Create(ctx, deployment)
		Expect(err).ToNot(HaveOccurred())

		// allow reconcile to happen
		time.Sleep(2 * time.Second)

		Eventually(func(g Gomega) error {
			nsName := types.NamespacedName{
				Namespace: namespace,
				Name:      deploymentName,
			}
			var updatedDepObj appsv1.Deployment
			err := k8sClient.Get(ctx, nsName, &updatedDepObj)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(updatedDepObj.Spec.Template.Annotations).NotTo(BeNil())
			restartedAt := updatedDepObj.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"]
			GinkgoWriter.Println("[test log] restartedAt", restartedAt)
			g.Expect(restartedAt).To(Equal(pastTime))
			return nil
		}).WithTimeout(waitTime).WithPolling(pollTime).Should(Succeed())

		err = k8sClient.Delete(ctx, deployment)
		Expect(err).ToNot(HaveOccurred())
	})

	It("updates matching criteria when flipper CR is deployed", func() {
		const deploymentMatch = "deployment-match"
		const deploymentNonMatch = "deployment-nonmatch"
		// create two deployment, one matching default criteria and one matching new criteria, only new should be updated
		flipper := &flipperiov1alpha1.Flipper{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo-flipper",
				Namespace: "default",
			},
			Spec: flipperiov1alpha1.FlipperSpec{
				Interval: metav1.Duration{Duration: 3 * time.Minute},
				Match: flipperiov1alpha1.MatchSpec{
					Labels: map[string]string{
						"foo": "true",
					},
				},
			},
		}
		err := k8sClient.Create(ctx, flipper)
		Expect(err).ToNot(HaveOccurred())

		// allow reconcile
		time.Sleep(2 * time.Second)

		depObjMatch := createDeployment(deploymentMatch, defaultns, "", map[string]string{"foo": "true"})
		err = k8sClient.Create(ctx, depObjMatch)
		Expect(err).ToNot(HaveOccurred())

		depObjNonMatch := createDeployment(deploymentNonMatch, defaultns, "", map[string]string{"mesh": "true"})
		err = k8sClient.Create(ctx, depObjNonMatch)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func(g Gomega) error {
			nsName := types.NamespacedName{
				Namespace: defaultns,
				Name:      deploymentMatch,
			}
			var updatedDepObj appsv1.Deployment
			err := k8sClient.Get(ctx, nsName, &updatedDepObj)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(updatedDepObj.Spec.Template.Annotations).NotTo(BeNil())
			restartedAt := updatedDepObj.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"]
			GinkgoWriter.Println("[test log] restartedAt", restartedAt)
			_, err = time.Parse(time.RFC3339, restartedAt)
			Expect(err).ToNot(HaveOccurred())

			return nil
		}).WithTimeout(waitTime).WithPolling(pollTime).Should(Succeed())

		Eventually(func(g Gomega) error {
			nsName := types.NamespacedName{
				Namespace: defaultns,
				Name:      deploymentNonMatch,
			}
			var updatedDepObj appsv1.Deployment
			err := k8sClient.Get(ctx, nsName, &updatedDepObj)
			g.Expect(err).ToNot(HaveOccurred())

			if updatedDepObj.Spec.Template.Annotations == nil {
				return nil
			}
			restartedAt := updatedDepObj.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"]
			g.Expect(restartedAt).To(BeEmpty())
			return nil
		}).WithTimeout(waitTime).WithPolling(pollTime).Should(Succeed())

		err = k8sClient.Delete(ctx, depObjMatch)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Delete(ctx, depObjNonMatch)
		Expect(err).ToNot(HaveOccurred())
	})

})

func createDeployment(name, namespace, restartAt string, labels map[string]string) *appsv1.Deployment {
	var annotations map[string]string
	if restartAt != "" {
		annotations = map[string]string{"kubectl.kubernetes.io/restartedAt": restartAt}
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"foo": "bar"},
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx",
						},
					},
				},
			},
		},
	}
}
