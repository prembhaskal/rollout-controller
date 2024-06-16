package rollout

import (
	"context"
	"time"

	flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/api/v1alpha1"
	"github.com/prembhaskal/rollout-controller/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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

func (r *Reconciler) enqueueDeploymentsForCriteriaChange(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	// TODO error in this method won't cause requeue, but chances of errors are less since it will be using cached clients for IO.
	var allDepls corev1.DeploymentList
	err := r.List(ctx, &allDepls)
	if err != nil {
		logger.Error(err, "error in listing deployments")
		return nil
	}
	requests := make([]reconcile.Request, 0)
	for _, depl := range allDepls.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      depl.Name,
				Namespace: depl.Namespace,
			},
		})
	}

	nsName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	var flipper flipperiov1alpha1.Flipper
	err = r.Get(ctx, nsName, &flipper)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("deleted matching criteria config")
			r.matchCriteria.DeleteConfig()
			return requests
		}
		logger.Error(err, "error in getting flipper configuration") // TODO this will never recover
		return nil
	}

	logger.Info("updated matching criteria", "flipper", flipper)
	r.matchCriteria.UpdateConfig(&flipper)
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Deployment{}).
		Named("rolloutController").
		Watches(&flipperiov1alpha1.Flipper{}, handler.EnqueueRequestsFromMapFunc(r.enqueueDeploymentsForCriteriaChange)).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
