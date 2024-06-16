package rollout

import (
	"context"
	"time"

	"github.com/prembhaskal/rollout-controller/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

var _ reconcile.Reconciler = &Reconciler{}

type Reconciler struct {
	client.Client
	matchCriteria *config.MatchCriteria
}

// const (
// 	matchLabel      = "mesh"
// 	matchValue      = "true"
// 	requeueInterval = 5 * time.Minute
// )

func New(client client.Client, matchCriteria *config.MatchCriteria) *Reconciler {
	return &Reconciler{
		Client:        client,
		matchCriteria: matchCriteria,
	}
}

// +kubebuilder:rbac:groups=apps.v1,resources=deployment,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=apps.v1,resources=deployment/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.v1,resources=deployment/finalizers,verbs=update

// Reconcile reconciles the deployment and triggers rollout restart if needed.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(2).Info("In Reconcile method")

	obj := &corev1.Deployment{}
	err := r.Get(ctx, req.NamespacedName, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.V(2).Info("Deployment deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to get deployment")
		return ctrl.Result{}, err
	}

	cfg := r.matchCriteria.Config()
	logger.V(0).Info("using", "matching config", cfg)

	if !r.matchesCriteria(obj, cfg) {
		logger.Info("ignoring non matching deployment")
		return ctrl.Result{}, nil
	}

	logger.Info("doing rollout restart for deployment...")

	objCopy := obj.DeepCopy()
	if objCopy.Spec.Template.ObjectMeta.Annotations == nil {
		objCopy.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	objCopy.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	err = r.Patch(ctx, objCopy, client.MergeFrom(obj))
	if err != nil {
		logger.Error(err, "error patching the deployment")
		return ctrl.Result{}, err
	}

	// TODO(user): your logic here
	return ctrl.Result{RequeueAfter: cfg.Interval}, nil
}

// check if obj matches the needed label and namespace
func (r *Reconciler) matchesCriteria(obj *corev1.Deployment, cfg config.FlipperConfig) bool {
	if obj.Spec.Template.Labels[cfg.MatchLabel] != cfg.MatchValue {
		return false
	}
	if cfg.Namespace != "" && cfg.Namespace != obj.Namespace {
		return false
	}
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Deployment{}).
		Named("rolloutController").
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
