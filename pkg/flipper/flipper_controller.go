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

package flipper

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/api/v1alpha1"
	"github.com/prembhaskal/rollout-controller/pkg/config"
)

// Reconciler reconciles a Flipper object
type Reconciler struct {
	client.Client
	scheme        *runtime.Scheme
	matchCriteria *config.MatchCriteria
}

func New(client client.Client, scheme *runtime.Scheme, matchCriteria *config.MatchCriteria) *Reconciler {
	return &Reconciler{
		Client:        client,
		scheme:        scheme,
		matchCriteria: matchCriteria,
	}
}

// +kubebuilder:rbac:groups=flipper.io.github.com,resources=flippers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=flipper.io.github.com,resources=flippers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=flipper.io.github.com,resources=flippers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Flipper object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.2/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("In Reconcile method")

	// TODO(user): your logic here
	obj := &flipperiov1alpha1.Flipper{}
	err := r.Get(ctx, req.NamespacedName, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("flipper config deleted")
			r.matchCriteria.DeleteConfig()
			return ctrl.Result{}, nil
		}
		logger.Error(err, "error fetching the flipper config")
		return ctrl.Result{}, err
	}

	logger.Info("updating matching criteria", "criteria", obj.Spec)
	r.matchCriteria.UpdateConfig(obj)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&flipperiov1alpha1.Flipper{}).
		Complete(r)
}
