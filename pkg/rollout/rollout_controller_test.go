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

	// flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/api/v1alpha1"
	"github.com/prembhaskal/rollout-controller/pkg/config"
	rollout "github.com/prembhaskal/rollout-controller/pkg/rollout"
)

var _ = Describe("Flipper Controller", Ordered, func() {
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
		// deployment := &appsv1.Deployment{
		// 	ObjectMeta: metav1.ObjectMeta{Name: "deployment-name", Namespace: "default"},
		// 	Spec: appsv1.DeploymentSpec{
		// 		Selector: &metav1.LabelSelector{
		// 			MatchLabels: map[string]string{"foo": "bar"},
		// 		},
		// 		Template: corev1.PodTemplateSpec{
		// 			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar", "mesh": "true"}},
		// 			Spec: corev1.PodSpec{
		// 				Containers: []corev1.Container{
		// 					{
		// 						Name:  "nginx",
		// 						Image: "nginx",
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// }
		const deploymentName = "deployment-one"
		const namespace = "default"
		deployment := createDeployment(deploymentName, namespace, "", map[string]string{"mesh":"true"})

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
		}).WithTimeout(30 * time.Second).WithPolling(5 * time.Second).Should(Succeed())
	})
})


func createDeployment(name, namespace, restartAt string, labels map[string]string) *appsv1.Deployment {
	var annotations map[string]string
	if restartAt != "" {
		annotations = map[string]string{"kubectl.kubernetes.io/restartedAt": restartAt}
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, 
			Namespace: namespace,
			Labels: labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"foo": "bar"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"foo": "bar"},
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