package rollout_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	gtypes "github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/api/v1alpha1"
	"github.com/prembhaskal/rollout-controller/pkg/config"
	"github.com/prembhaskal/rollout-controller/pkg/rollout"
)

func TestRolloutController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller unit Tests", gtypes.ReporterConfig{Verbose: false})
}

var _ = Describe("Flipper Controller", Ordered, func() {
	ctx := context.Background()
	// setup fake client with needed parameters
	client := createFakeClient()
	// setup default match criteria (or read from somewhere)
	matchCriteria := &config.MatchCriteria{}
	reconciler := rollout.New(client, matchCriteria)

	var depObject *appsv1.Deployment

	// clean up deployments
	AfterEach(func() {
		if depObject != nil {
			var obj appsv1.Deployment
			nsName := types.NamespacedName{Namespace: depObject.Namespace, Name: depObject.Name}
			err := client.Get(ctx, nsName, &obj)
			if err != nil && errors.IsNotFound(err) {
				return
			}
			Expect(err).ToNot(HaveOccurred())
			// remove finalizers and delete
			if len(obj.Finalizers) > 0 {
				obj.Finalizers = nil
				err = client.Update(ctx, &obj)
				Expect(err).ToNot(HaveOccurred())
			}
			client.Delete(ctx, &obj)
			depObject = nil
		}
	})

	Context("Matching Deployments", func() {
		It("Restarts Matching Deployment and Requeue", func() {
			// create a deployment matching the default criteria
			previousRestart := time.Now().Add(-11 * time.Minute).Format(time.RFC3339)
			depObject = createTestDeployment("orders", "random", previousRestart, map[string]string{"mesh": "true"})
			err := client.Create(ctx, depObject)
			Expect(err).ToNot(HaveOccurred())

			// reconcile
			nsName := types.NamespacedName{Name: "orders", Namespace: "random"}
			req := ctrl.Request{NamespacedName: nsName}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// restart and requeue for next rounds
			var obj appsv1.Deployment
			err = client.Get(ctx, nsName, &obj)
			Expect(err).ToNot(HaveOccurred())

			restartedAt := getRestartedAt(&obj)
			Expect(restartedAt).ToNot(BeEmpty())

			_, err = time.Parse(time.RFC3339, restartedAt)
			Expect(err).ToNot(HaveOccurred())
			Expect(restartedAt).ToNot(Equal(previousRestart))

			Expect(result.RequeueAfter).To(Equal(matchCriteria.Config().Interval))
		})

		It("Does not Restart Fresh Deployment immediately", func() {
			// create a deployment matching the default criteria
			depObject = createTestDeployment("orders", "random", "", map[string]string{"mesh": "true"})
			err := client.Create(ctx, depObject)
			Expect(err).ToNot(HaveOccurred())

			// reconcile
			nsName := types.NamespacedName{Name: "orders", Namespace: "random"}
			req := ctrl.Request{NamespacedName: nsName}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// restart and requeue for next rounds
			var obj appsv1.Deployment
			err = client.Get(ctx, nsName, &obj)
			Expect(err).ToNot(HaveOccurred())

			restartedAt := getRestartedAt(&obj)
			Expect(restartedAt).To(BeEmpty())

			Expect(result.RequeueAfter).To(Equal(matchCriteria.Config().Interval))
		})

		It("Does not Restart Deployment previously restarted in current interval", func() {
			now := time.Now()
			previousRestart := now.Add(-2 * time.Minute)
			prevRestartStr := previousRestart.Format(time.RFC3339)

			// create a deployment with previous restart time within current interval
			depObject = createTestDeployment("orders", "random", prevRestartStr, map[string]string{"mesh": "true"})
			err := client.Create(ctx, depObject)
			Expect(err).ToNot(HaveOccurred())

			// reconcile
			nsName := types.NamespacedName{Name: "orders", Namespace: "random"}
			req := ctrl.Request{NamespacedName: nsName}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// restart and requeue for next rounds
			var obj appsv1.Deployment
			err = client.Get(ctx, nsName, &obj)
			Expect(err).ToNot(HaveOccurred())

			restartedAt := getRestartedAt(&obj)
			Expect(restartedAt).To(Equal(prevRestartStr))

			// restart after around 10-2=8 mins so < 10mins.
			GinkgoWriter.Printf("requeue After: %d", result.RequeueAfter)
			Expect(result.RequeueAfter < matchCriteria.Config().Interval).To(BeTrue())
		})

		It("handles improperly formatted lastRestart time", func() {
			now := time.Now()
			previousRestart := now.Add(-10 * time.Minute)
			prevRestartStr := previousRestart.Format(time.RFC1123)

			// create a deployment with previous restart time within current interval
			depObject = createTestDeployment("orders", "random", prevRestartStr, map[string]string{"mesh": "true"})
			err := client.Create(ctx, depObject)
			Expect(err).ToNot(HaveOccurred())

			// reconcile
			nsName := types.NamespacedName{Name: "orders", Namespace: "random"}
			req := ctrl.Request{NamespacedName: nsName}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.RequeueAfter).To(Equal(matchCriteria.Config().Interval))
		})

		It("requeues deployment in next interval whose previous restart time is set in future", func() {
			now := time.Now()
			previousRestart := now.Add(5 * time.Minute)
			prevRestartStr := previousRestart.Format(time.RFC3339)

			// create a deployment with previous restart time within current interval
			depObject = createTestDeployment("orders", "random", prevRestartStr, map[string]string{"mesh": "true"})
			err := client.Create(ctx, depObject)
			Expect(err).ToNot(HaveOccurred())

			// reconcile
			nsName := types.NamespacedName{Name: "orders", Namespace: "random"}
			req := ctrl.Request{NamespacedName: nsName}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// restart and requeue for next rounds
			var obj appsv1.Deployment
			err = client.Get(ctx, nsName, &obj)
			Expect(err).ToNot(HaveOccurred())

			restartedAt := getRestartedAt(&obj)
			Expect(restartedAt).To(Equal(prevRestartStr))

			Expect(result.RequeueAfter).To(Equal(matchCriteria.Config().Interval))

		})
	})

	Context("Non Matching deployments", func() {
		It("ignores deployment with non matching label", func() {
			// create a deployment matching the default criteria
			previousRestart := time.Now().Add(-11 * time.Minute).Format(time.RFC3339)
			depObject = createTestDeployment("orders", "random", previousRestart, map[string]string{"mesh": "false"})
			err := client.Create(ctx, depObject)
			Expect(err).ToNot(HaveOccurred())

			// reconcile
			nsName := types.NamespacedName{Name: "orders", Namespace: "random"}
			req := ctrl.Request{NamespacedName: nsName}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// restart and requeue for next rounds
			var obj appsv1.Deployment
			err = client.Get(ctx, nsName, &obj)
			Expect(err).ToNot(HaveOccurred())

			restartedAt := getRestartedAt(&obj)
			Expect(restartedAt).To(Equal(previousRestart))

			Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
		})
	})

	Context("Updated Flipper Config", func() {
		BeforeEach(func() {
			newFlipperCR := getFlipperCR("foo-flipper", "service", 3*time.Minute, map[string]string{"foo": "bar"})
			matchCriteria.UpdateConfig(newFlipperCR)
		})
		AfterEach(func() {
			matchCriteria.DeleteConfig()
		})
		It("matches new labels", func() {
			// create a deployment matching the default criteria
			previousRestart := time.Now().Add(-11 * time.Minute).Format(time.RFC3339)
			depObject = createTestDeployment("orders", "service", previousRestart, map[string]string{"foo": "bar"})
			err := client.Create(ctx, depObject)
			Expect(err).ToNot(HaveOccurred())

			// reconcile
			nsName := types.NamespacedName{Name: "orders", Namespace: "service"}
			req := ctrl.Request{NamespacedName: nsName}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// restart and requeue for next rounds
			var obj appsv1.Deployment
			err = client.Get(ctx, nsName, &obj)
			Expect(err).ToNot(HaveOccurred())

			restartedAt := getRestartedAt(&obj)
			Expect(restartedAt).ToNot(BeEmpty())
			Expect(restartedAt).ToNot(Equal(previousRestart))

			_, err = time.Parse(time.RFC3339, restartedAt)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.RequeueAfter).To(Equal(matchCriteria.Config().Interval))
		})

		It("ignores deployment in different namespace", func() {
			// create a deployment matching the default criteria
			previousRestart := time.Now().Add(-11 * time.Minute).Format(time.RFC3339)
			depObject = createTestDeployment("orders", "random", previousRestart, map[string]string{"foo": "bar"})
			err := client.Create(ctx, depObject)
			Expect(err).ToNot(HaveOccurred())

			// reconcile
			nsName := types.NamespacedName{Name: "orders", Namespace: "random"}
			req := ctrl.Request{NamespacedName: nsName}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// restart and requeue for next rounds
			var obj appsv1.Deployment
			err = client.Get(ctx, nsName, &obj)
			Expect(err).ToNot(HaveOccurred())

			restartedAt := getRestartedAt(&obj)
			Expect(restartedAt).To(Equal(previousRestart))

			Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
		})

	})

	Context("Deleted or Deleting deployments", func() {
		It("ignores deployment not existing", func() {
			// reconcile
			nsName := types.NamespacedName{Name: "deleted", Namespace: "random"}
			req := ctrl.Request{NamespacedName: nsName}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
		})

		It("ignores deployment in deleting state", func() {
			// create a deployment matching the default criteria
			depObject = createTestDeployment("deleting", "random", "", map[string]string{"mesh": "true"})
			depObject.Finalizers = append(depObject.Finalizers, "pending-delete")
			err := client.Create(ctx, depObject)
			Expect(err).ToNot(HaveOccurred())

			// mark it deleting, object won't be really deleted because of finalizer above.
			client.Delete(ctx, depObject)

			// reconcile
			nsName := types.NamespacedName{Name: "deleting", Namespace: "random"}
			req := ctrl.Request{NamespacedName: nsName}
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// restart and requeue for next rounds
			var obj appsv1.Deployment
			err = client.Get(ctx, nsName, &obj)
			Expect(err).ToNot(HaveOccurred())

			restartedAt := getRestartedAt(&obj)
			Expect(restartedAt).To(BeEmpty())

			Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
		})
	})
})

func getFlipperCR(name, matchnamespace string, requeueInterval time.Duration, labels map[string]string) *flipperiov1alpha1.Flipper {
	return &flipperiov1alpha1.Flipper{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: flipperiov1alpha1.FlipperSpec{
			Interval: metav1.Duration{Duration: requeueInterval},
			Match: flipperiov1alpha1.MatchSpec{
				Labels:     labels,
				Namespaces: []string{matchnamespace},
			},
		},
	}
}

func createTestDeployment(name, namespace, lastRestartAt string, labels map[string]string) *appsv1.Deployment {
	var annotations, rolloutlastRestartAnnotation map[string]string
	if lastRestartAt != "" {
		annotations = map[string]string{"kubectl.kubernetes.io/restartedAt": lastRestartAt}
		rolloutlastRestartAnnotation = map[string]string{rollout.RolloutLastRestartAnnotation: lastRestartAt}
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: rolloutlastRestartAnnotation,
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

func createFakeClient() client.Client {
	return fake.NewClientBuilder().
		Build()
}

func getRestartedAt(obj *appsv1.Deployment) string {
	annotations := obj.Spec.Template.Annotations
	if annotations == nil {
		return ""
	}
	return annotations["kubectl.kubernetes.io/restartedAt"]
}
