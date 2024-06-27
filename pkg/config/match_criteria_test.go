package config_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/pkg/api/flipper/v1alpha1"
	"github.com/prembhaskal/rollout-controller/pkg/config"
)

func TestRolloutController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Match Criteria unit Tests")
}

var _ = Describe("Match Criteria Tests", func() {
	It("uses default config on new", func() {
		matchCriteria := config.MatchCriteria{}
		config := matchCriteria.Config()
		Expect(config.Interval).To(Equal(time.Duration(10 * time.Minute)))
		Expect(config.MatchLabels).To(HaveLen(1))
		Expect(config.MatchLabels["mesh"]).To(Equal("true"))
		Expect(config.Namespaces).To(BeEmpty())
	})

	It("matches any one of the label", func() {
		expLabels := map[string]string{"foo": "bar", "mesh": "true"}
		flipper := createFlipperCR("test", []string{"data"}, time.Duration(2*time.Minute), expLabels)
		matchCriteria := config.MatchCriteria{}
		matchCriteria.UpdateConfig(flipper)

		actLabels := map[string]string{"app": "orders", "mesh": "true"}
		obj := createTestDeployment("orders", "data", actLabels)

		Expect(matchCriteria.Matches(obj)).To(BeTrue())
	})

	It("matches object in any namespace when config has empty namespace", func() {
		expLabels := map[string]string{"mesh": "true"}
		flipper := createFlipperCR("test", []string{}, time.Duration(2*time.Minute), expLabels)
		matchCriteria := config.MatchCriteria{}
		matchCriteria.UpdateConfig(flipper)

		actLabels := map[string]string{"app": "orders", "mesh": "true"}
		obj := createTestDeployment("orders", "data", actLabels)

		Expect(matchCriteria.Matches(obj)).To(BeTrue())
	})

	It("matches any one of the namespaces", func() {
		expLabels := map[string]string{"mesh": "true"}
		flipper := createFlipperCR("test", []string{"data", "inventory"}, time.Duration(2*time.Minute), expLabels)
		matchCriteria := config.MatchCriteria{}
		matchCriteria.UpdateConfig(flipper)

		actLabels := map[string]string{"app": "orders", "mesh": "true"}
		obj := createTestDeployment("orders", "data", actLabels)

		Expect(matchCriteria.Matches(obj)).To(BeTrue())
	})

	It("does not matches none of labels match", func() {
		expLabels := map[string]string{"foo": "bar", "mesh": "true"}
		flipper := createFlipperCR("test", []string{"data"}, time.Duration(2*time.Minute), expLabels)
		matchCriteria := config.MatchCriteria{}
		matchCriteria.UpdateConfig(flipper)

		actLabels := map[string]string{"app": "orders", "mesh": "false"}
		obj := createTestDeployment("orders", "data", actLabels)

		Expect(matchCriteria.Matches(obj)).To(BeFalse())
	})

	It("does not matche none of namespaces match", func() {
		expLabels := map[string]string{"mesh": "true"}
		flipper := createFlipperCR("test", []string{"data"}, time.Duration(2*time.Minute), expLabels)
		matchCriteria := config.MatchCriteria{}
		matchCriteria.UpdateConfig(flipper)

		actLabels := map[string]string{"app": "orders", "mesh": "true"}
		obj := createTestDeployment("orders", "search", actLabels)

		Expect(matchCriteria.Matches(obj)).To(BeFalse())
	})

})

func createTestDeployment(name, namespace string, labels map[string]string) *appsv1.Deployment {
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
					Labels: map[string]string{"foo": "bar"},
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

func createFlipperCR(name string, matchnamespaces []string, requeueInterval time.Duration, labels map[string]string) *flipperiov1alpha1.Flipper {
	return &flipperiov1alpha1.Flipper{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: flipperiov1alpha1.FlipperSpec{
			Interval: metav1.Duration{Duration: requeueInterval},
			Match: flipperiov1alpha1.MatchSpec{
				Labels:     labels,
				Namespaces: matchnamespaces,
			},
		},
	}
}
