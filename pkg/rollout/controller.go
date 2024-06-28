package rollout

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/pkg/api/flipper/v1alpha1"
	"github.com/prembhaskal/rollout-controller/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
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

const (
	RolloutLastRestartAnnotation = "flipper.io/rollout-last-restart"
	RestartedAtAnnotation        = "kubectl.kubernetes.io/restartedAt"
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

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=flipper.io.github.com,resources=flippers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=flipper.io.github.com,resources=flippers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=flipper.io.github.com,resources=flippers/finalizers,verbs=update

// Reconcile reconciles the deployment and triggers rollout restart if needed.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx) // default level as 2
	logger.Info("In Reconcile method")

	obj := &appsv1.Deployment{}
	err := r.Get(ctx, req.NamespacedName, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Deployment deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to get deployment")
		return ctrl.Result{}, err
	}

	// if already deleting, ignore it
	if obj.DeletionTimestamp != nil {
		logger.Info("Ignoring deployment in deleting state")
		return ctrl.Result{}, nil
	}

	cfg, match := r.matchCriteria.MatchingConfig(obj)
	if !match {
		logger.Info("ignoring non matching deployment")
		return ctrl.Result{}, nil
	}

	currRestartTime := time.Now()
	restartnow, result, err := r.shouldRestartNow(ctx, logger, obj, currRestartTime, cfg.Interval)
	if !restartnow {
		if err == nil {
			logger.Info("skipping as restart not needed now, will be tried in nextInterval",
				"nextInterval", result.RequeueAfter)
		}
		return result, err
	}

	logger.Info("performing rollout restart for deployment...")
	err = r.dotriggerRollout(ctx, obj, currRestartTime)
	if err != nil {
		logger.Error(err, "error patching the deployment")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: cfg.Interval}, nil
}

func (r *Reconciler) dotriggerRollout(ctx context.Context, obj *appsv1.Deployment, restartTime time.Time) error {
	restartTimeFormatted := restartTime.Format(time.RFC3339)

	objcopy := obj.DeepCopy()
	if objcopy.Spec.Template.ObjectMeta.Annotations == nil {
		objcopy.Spec.Template.ObjectMeta.Annotations = make(map[string]string)
	}
	objcopy.Spec.Template.ObjectMeta.Annotations[RestartedAtAnnotation] = restartTimeFormatted

	if objcopy.Annotations == nil {
		objcopy.Annotations = make(map[string]string)
	}
	objcopy.Annotations[RolloutLastRestartAnnotation] = restartTimeFormatted

	return r.Patch(ctx, objcopy, client.MergeFrom(obj))
}

func (r *Reconciler) updateRolloutLastRestartAnnotation(ctx context.Context, obj *appsv1.Deployment) error {
	objcopy := obj.DeepCopy()
	if objcopy.Annotations == nil {
		objcopy.Annotations = make(map[string]string)
	}
	objcopy.Annotations[RolloutLastRestartAnnotation] = time.Now().Format(time.RFC3339)

	return r.Patch(ctx, objcopy, client.MergeFrom(obj))
}

// returns true if it should be restarted now.
// return false and reconcile result with requeuAfter set if we cannot restart now
// it also updates the rollout last restart annotation in case it was empty or previously set incorrectly.
// it returns the error if update fails
func (r *Reconciler) shouldRestartNow(ctx context.Context, logger logr.Logger,
	obj *appsv1.Deployment, currRestartTime time.Time,
	restartInterval time.Duration) (bool, ctrl.Result, error) {

	// check if it is first seen by controller
	// if rollout restart absent , update to now and requeue for next interval
	rolloutLastRestart := getRolloutLastRestart(obj)
	if rolloutLastRestart == "" {
		err := r.updateRolloutLastRestartAnnotation(ctx, obj)
		if err != nil {
			logger.Error(err, "error adding rollout last restart annotation")
			return false, ctrl.Result{}, err
		}
		return false, ctrl.Result{RequeueAfter: restartInterval}, nil
	}

	// if rollout time present and invalid, fix it to now and requeue for next interval
	lastRestarted, err := time.Parse(time.RFC3339, rolloutLastRestart)
	if err != nil {
		err = r.updateRolloutLastRestartAnnotation(ctx, obj)
		if err != nil {
			logger.Error(err, "error adding rollout restart annotation")
			return false, ctrl.Result{}, err
		}
		return false, ctrl.Result{RequeueAfter: restartInterval}, nil
	}

	expPrevRestart := currRestartTime.Add(-restartInterval)
	// exp ... now ... last (set in future)
	if lastRestarted.After(currRestartTime) {
		return false, ctrl.Result{RequeueAfter: restartInterval}, nil
	}
	// exp ... last ... now ... <next-int> ... nextRestart
	if expPrevRestart.Before(lastRestarted) {
		nextInterval := restartInterval - currRestartTime.Sub(lastRestarted)
		return false, ctrl.Result{RequeueAfter: nextInterval}, nil
	}

	return true, ctrl.Result{}, nil
}

func getRolloutLastRestart(obj *appsv1.Deployment) string {
	if obj.Annotations == nil {
		return ""
	}
	return obj.Annotations[RolloutLastRestartAnnotation]
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
			NamespacedName: types.NamespacedName{Name: depl.Name, Namespace: depl.Namespace},
		})
	}

	nsName := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}
	var flipper flipperiov1alpha1.Flipper
	err = r.Get(ctx, nsName, &flipper)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("deleted matching criteria config")
			r.matchCriteria.DeleteConfig(obj.GetName())
			return requests
		}
		logger.Error(err, "error in getting flipper configuration")
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
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}
