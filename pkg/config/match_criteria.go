package config

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/pkg/api/flipper/v1alpha1"
	flipperclient "github.com/prembhaskal/rollout-controller/pkg/client/versioned"
)

type MatchCriteria struct {
	config *FlipperConfig
	mu     sync.Mutex
}

type FlipperConfig struct {
	MatchLabels map[string]string
	Interval    time.Duration
	Namespaces  []string
}

func (f FlipperConfig) String() string {
	return fmt.Sprintf("match labels [%v] every [%s] in namespaces %v",
		f.MatchLabels, f.Interval.String(), f.Namespaces)
}

var defaultConfig = FlipperConfig{
	MatchLabels: map[string]string{"mesh": "true"},
	Interval:    10 * time.Minute,
	Namespaces:  []string{},
}

func (m *MatchCriteria) Config() FlipperConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.config == nil {
		return defaultConfig
	}
	return *m.config
}

// LoadFlipperConfig read flipper CRs from cluster and uses one of that to update
// the configuration for match criteria
func (m *MatchCriteria) LoadFlipperConfig(restConfig *rest.Config) error {
	client, err := flipperclient.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	flipperCRs, err := client.FlipperV1alpha1().Flippers("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	if len(flipperCRs.Items) == 0 {
		return nil
	}
	// read the first one
	m.UpdateConfig(&flipperCRs.Items[0])
	return nil
}

// TODO implement
func (m *MatchCriteria) Validate(flip *flipperiov1alpha1.Flipper) error {
	return nil
}

func (m *MatchCriteria) UpdateConfig(flip *flipperiov1alpha1.Flipper) {
	m.mu.Lock()
	defer m.mu.Unlock()

	matchLabels := make(map[string]string)
	for k, v := range flip.Spec.Match.Labels {
		matchLabels[k] = v
	}

	m.config = &FlipperConfig{
		MatchLabels: matchLabels,
		Interval:    flip.Spec.Interval.Duration,
		Namespaces:  flip.Spec.Match.Namespaces,
	}
}

func (m *MatchCriteria) DeleteConfig() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = nil
}

func (m *MatchCriteria) Matches(obj metav1.Object) bool {
	cfg := m.Config()
	// match all labels
	if !matchLabels(obj.GetLabels(), cfg.MatchLabels) {
		return false
	}
	if !matchNamespaces(obj.GetNamespace(), cfg.Namespaces) {
		return false
	}
	return true
}

// return true if actual matches with any one of the expected namespaces
func matchNamespaces(act string, exp []string) bool {
	// empty means we match all namespaces
	if len(exp) == 0 {
		return true
	}
	for _, ns := range exp {
		if ns == act {
			return true
		}
	}
	return false
}

// return true if actual map has at least one matching entry from expected map
// or if expected map is empty
func matchLabels(act, exp map[string]string) bool {
	// if no expected labels, then it matches everything.
	if len(exp) == 0 {
		return true
	}
	for k, v := range act {
		if exp[k] == v {
			return true
		}
	}
	return false
}
