package rollout

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/api/v1alpha1"
	"github.com/prembhaskal/rollout-controller/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var _ reconcile.Reconciler = &Reconciler{}

type Reconciler struct {
	client.Client
	matchCriteria *config.MatchCriteria
}

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

	obj := &appsv1.Deployment{}
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

	restartTime := time.Now()
	restartNeeded, nextInterval, err := r.isRestartNeeded(logger, obj, restartTime, cfg.Interval)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !restartNeeded {
		logger.Info("skipping deployment as restart not needed now, will be tried in nextInterval", "nextInterval", nextInterval)
		return ctrl.Result{RequeueAfter: nextInterval}, nil
	}

	logger.Info("doing rollout restart for deployment...")
	err = r.triggerRollout(ctx, obj, restartTime)
	if err != nil {
		logger.Error(err, "error patching the deployment")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: cfg.Interval}, nil
}

func (r *Reconciler) triggerRollout(ctx context.Context, obj *appsv1.Deployment, restartTime time.Time) error {
	objCopy := obj.DeepCopy()
	if objCopy.Spec.Template.ObjectMeta.Annotations == nil {
		objCopy.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	objCopy.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"] = restartTime.Format(time.RFC3339)
	return r.Patch(ctx, objCopy, client.MergeFrom(obj))
}

// return true if restart needed
// return false with error if issue finding previous restart time
// returns false and interval after which restart to be triggered
func (r *Reconciler) isRestartNeeded(logger logr.Logger, obj *appsv1.Deployment, restartTime time.Time, restartInterval time.Duration) (bool, time.Duration, error) {
	if obj.Spec.Template.ObjectMeta.Annotations == nil {
		return true, 0, nil
	}
	lastRestartedStr := obj.Spec.Template.ObjectMeta.Annotations["kubectl.kubernetes.io/restartedAt"]
	if lastRestartedStr == "" {
		return true, 0, nil
	}
	lastRestarted, err := time.Parse(time.RFC3339, lastRestartedStr)
	if err != nil {
		logger.Error(err, "error parsing last restart time from deployment", "lastRestarted", lastRestarted)
		return false, 0, err
	}
	if restartTime.Before(lastRestarted) {
		// this can happen if someone manually edits deployment incorrectly
		logger.Info("error: last restart time is in future", "lastRestart", lastRestarted, "newRestart", restartTime)
		return false, 0, err
	}
	nextRestartInterval := restartInterval - restartTime.Sub(lastRestarted)
	// lastRestart + restartInterval < newRestartTime <-- match this condition for restart
	return lastRestarted.Add(restartInterval).Before(restartTime), nextRestartInterval, nil
}

// check if obj matches the needed label and namespace
func (r *Reconciler) matchesCriteria(obj *appsv1.Deployment, cfg config.FlipperConfig) bool {
	if obj.Labels[cfg.MatchLabel] != cfg.MatchValue {
		return false
	}
	if cfg.Namespace != "" && cfg.Namespace != obj.Namespace {
		return false
	}
	return true
}

func (r *Reconciler) enqueueDeploymentsForCriteriaChange(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	// TODO error in this method won't cause requeue,
	// but chances of errors are less since it will be using cached clients.
	var allDepls appsv1.DeploymentList
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
		For(&appsv1.Deployment{}).
		Named("rolloutController").
		Watches(&flipperiov1alpha1.Flipper{}, handler.EnqueueRequestsFromMapFunc(r.enqueueDeploymentsForCriteriaChange)).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
