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

func TestMatchCriteria(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Match Criteria unit Tests")
}

var _ = Describe("Match Criteria Tests", func() {
	It("uses default config on new", func() {
		matchCriteria := config.NewMatchCriteria()
		actLabels := map[string]string{"mesh": "true"}
		obj := createTestDeployment("orders", "", actLabels)
		config, match := matchCriteria.MatchingConfig(obj)
		Expect(match).To(BeTrue())
		Expect(config.Interval).To(Equal(time.Duration(10 * time.Minute)))
		Expect(config.MatchLabels).To(HaveLen(1))
		Expect(config.MatchLabels["mesh"]).To(Equal("true"))
		Expect(config.Namespaces).To(BeEmpty())
	})

	It("matches any one of the label", func() {
		expLabels := map[string]string{"foo": "bar", "mesh": "true"}
		flipper := createFlipperCR("test", []string{"data"}, time.Duration(2*time.Minute), expLabels)
		matchCriteria := config.NewMatchCriteria()
		matchCriteria.UpdateConfig(flipper)

		actLabels := map[string]string{"app": "orders", "mesh": "true"}
		obj := createTestDeployment("orders", "data", actLabels)

		cfg, matched := matchCriteria.MatchingConfig(obj)
		Expect(matched).To(BeTrue())
		Expect(cfg.Name).To(Equal(flipper.Name))
	})

	It("matches object with appropriate configuration among many", func() {
		matchCriteria := config.NewMatchCriteria()

		meshLabels := map[string]string{"foo": "bar", "mesh": "true"}
		meshFlipper := createFlipperCR("mesh-flipper", []string{"data"}, time.Duration(2*time.Minute), meshLabels)
		matchCriteria.UpdateConfig(meshFlipper)

		nonMeshLabels := map[string]string{"foo": "bar", "mesh": "false"}
		nonMeshFlipper := createFlipperCR("non-mesh-flipper", []string{"payment"}, time.Duration(5*time.Minute), nonMeshLabels)
		matchCriteria.UpdateConfig(nonMeshFlipper)

		actLabels := map[string]string{"app": "orders", "mesh": "true"}
		obj := createTestDeployment("orders", "data", actLabels)

		cfg, matched := matchCriteria.MatchingConfig(obj)
		Expect(matched).To(BeTrue())
		Expect(cfg.Name).To(Equal(meshFlipper.Name))

	})

	It("matches object in any namespace when config has empty namespace", func() {
		expLabels := map[string]string{"mesh": "true"}
		flipper := createFlipperCR("test", []string{}, time.Duration(2*time.Minute), expLabels)
		matchCriteria := config.NewMatchCriteria()
		matchCriteria.UpdateConfig(flipper)

		actLabels := map[string]string{"app": "orders", "mesh": "true"}
		obj := createTestDeployment("orders", "data", actLabels)

		cfg, matched := matchCriteria.MatchingConfig(obj)
		Expect(matched).To(BeTrue())
		Expect(cfg.Name).To(Equal(flipper.Name))
	})

	It("matches any one of the namespaces", func() {
		expLabels := map[string]string{"mesh": "true"}
		flipper := createFlipperCR("test", []string{"data", "inventory"}, time.Duration(2*time.Minute), expLabels)
		matchCriteria := config.NewMatchCriteria()
		matchCriteria.UpdateConfig(flipper)

		actLabels := map[string]string{"app": "orders", "mesh": "true"}
		obj := createTestDeployment("orders", "data", actLabels)

		cfg, matched := matchCriteria.MatchingConfig(obj)
		Expect(matched).To(BeTrue())
		Expect(cfg.Name).To(Equal(flipper.Name))
	})

	It("does not matches none of labels match", func() {
		expLabels := map[string]string{"foo": "bar", "mesh": "true"}
		flipper := createFlipperCR("test", []string{"data"}, time.Duration(2*time.Minute), expLabels)
		matchCriteria := config.NewMatchCriteria()
		matchCriteria.UpdateConfig(flipper)

		actLabels := map[string]string{"app": "orders", "mesh": "false"}
		obj := createTestDeployment("orders", "data", actLabels)

		_, matched := matchCriteria.MatchingConfig(obj)
		Expect(matched).To(BeFalse())
	})

	It("does not matche none of namespaces match", func() {
		expLabels := map[string]string{"mesh": "true"}
		flipper := createFlipperCR("test", []string{"data"}, time.Duration(2*time.Minute), expLabels)
		matchCriteria := config.NewMatchCriteria()
		matchCriteria.UpdateConfig(flipper)

		actLabels := map[string]string{"app": "orders", "mesh": "true"}
		obj := createTestDeployment("orders", "search", actLabels)

		_, matched := matchCriteria.MatchingConfig(obj)
		Expect(matched).To(BeFalse())
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
