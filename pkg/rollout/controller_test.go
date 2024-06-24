package rollout_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/prembhaskal/rollout-controller/pkg/config"
	"github.com/prembhaskal/rollout-controller/pkg/rollout"
)

func createTestDeployment(name, namespace, restartAt string, labels map[string]string) *appsv1.Deployment {
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

func createFakeClient() client.Client {
	return fake.NewClientBuilder().
		WithObjects(createTestDeployment("orders", "default", "", map[string]string{"mesh": "true"})).
		Build()
}

func TestReconcilerRestartWithRequeue(t *testing.T) {
	ctx := context.Background()
	// setup fake client with needed parameters
	client := createFakeClient()
	// setup default match criteria (or read from somewhere)
	matchCriteria := &config.MatchCriteria{}
	reconciler := rollout.New(client, matchCriteria)

	nsName := types.NamespacedName{
		Namespace: "default",
		Name:      "orders",
	}
	req := ctrl.Request{
		NamespacedName: nsName,
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	var obj appsv1.Deployment
	err = client.Get(ctx, nsName, &obj)
	require.NoError(t, err)

	annotations := obj.Spec.Template.Annotations
	require.NotNil(t, annotations)
	restartedAt := annotations["kubectl.kubernetes.io/restartedAt"]
	require.NotEmpty(t, restartedAt)

	_, err = time.Parse(time.RFC3339, restartedAt)
	require.NoError(t, err)

	require.Equal(t, result.RequeueAfter, matchCriteria.Config().Interval)
}

func TestReconcilerIgnoreNotMatchingLabel(t *testing.T) {
	ctx := context.Background()
	// setup fake client with needed parameters
	client := createFakeClient()
	// add non matching deployment
	nonMatchLabelDeployment := createTestDeployment("booking", "default", "", map[string]string{"mesh": "false"})
	client.Create(ctx, nonMatchLabelDeployment)
	// setup default match criteria (or read from somewhere)
	matchCriteria := &config.MatchCriteria{}
	reconciler := rollout.New(client, matchCriteria)

	nsName := types.NamespacedName{
		Namespace: "default",
		Name:      "booking",
	}
	req := ctrl.Request{
		NamespacedName: nsName,
	}

	result, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	var obj appsv1.Deployment
	err = client.Get(ctx, nsName, &obj)
	require.NoError(t, err)

	annotations := obj.Spec.Template.Annotations
	
	require.NotNil(t, annotations)
	restartedAt := annotations["kubectl.kubernetes.io/restartedAt"]
	require.Empty(t, restartedAt)

	_, err = time.Parse(time.RFC3339, restartedAt)
	require.NoError(t, err)

	require.Equal(t, result.RequeueAfter, matchCriteria.Config().Interval)
}