//go:build integration
// +build integration

package rollout_test

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/pkg/api/v1alpha1"
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

	It("Restarts a deployment matching labels", func() {
		const resourceName = "test-resource"
		const deploymentName = "deployment-one"
		const namespace = "default"
		previousRestart := time.Now().Add(-11 * time.Minute).Format(time.RFC3339)
		deployment := createTestDeployment(deploymentName, namespace, previousRestart, map[string]string{"mesh": "true"})

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

			restartedAt := getRestartedAt(&updatedDepObj)
			GinkgoWriter.Println("[test log] restartedAt", restartedAt)
			g.Expect(restartedAt).ToNot(BeEmpty())
			g.Expect(restartedAt).ToNot(Equal(previousRestart))

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
		previousRestart := time.Now().Add(-11 * time.Minute).Format(time.RFC3339)
		deployment := createTestDeployment(deploymentName, namespace, previousRestart, map[string]string{"mesh": "false"})

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

			restartedAt := getRestartedAt(&updatedDepObj)
			g.Expect(restartedAt).To(Equal(previousRestart))
			return nil
		}).WithTimeout(waitTime).WithPolling(pollTime).Should(Succeed())

		err = k8sClient.Delete(ctx, deployment)
		Expect(err).ToNot(HaveOccurred())
	})

	It("does not restart a deployment which was already restarted in past interval", func() {
		const resourceName = "test-resource"
		const deploymentName = "deployment-three"
		const namespace = "default"

		previousRestart := time.Now().Add(-1 * time.Minute).Format(time.RFC3339)
		GinkgoWriter.Println("[test log] currTime", previousRestart)
		deployment := createTestDeployment(deploymentName, namespace, previousRestart, map[string]string{"mesh": "true"})

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

			restartedAt := getRestartedAt(&updatedDepObj)
			GinkgoWriter.Println("[test log] restartedAt", restartedAt)
			g.Expect(restartedAt).To(Equal(previousRestart))
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

		// allow flipper CR reconcile
		time.Sleep(2 * time.Second)
		previousRestart := time.Now().Add(-11 * time.Minute).Format(time.RFC3339)
		depObjMatch := createTestDeployment(deploymentMatch, defaultns, previousRestart, map[string]string{"foo": "true"})
		err = k8sClient.Create(ctx, depObjMatch)
		Expect(err).ToNot(HaveOccurred())

		depObjNonMatch := createTestDeployment(deploymentNonMatch, defaultns, previousRestart, map[string]string{"mesh": "true"})
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

			restartedAt := getRestartedAt(&updatedDepObj)
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

			restartedAt := getRestartedAt(&updatedDepObj)
			g.Expect(restartedAt).To(Equal(previousRestart))
			return nil
		}).WithTimeout(waitTime).WithPolling(pollTime).Should(Succeed())

		err = k8sClient.Delete(ctx, depObjMatch)
		Expect(err).ToNot(HaveOccurred())
		err = k8sClient.Delete(ctx, depObjNonMatch)
		Expect(err).ToNot(HaveOccurred())
	})

})
