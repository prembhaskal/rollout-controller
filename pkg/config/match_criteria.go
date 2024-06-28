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

type FlipperConfig struct {
	Name        string
	MatchLabels map[string]string
	Interval    time.Duration
	Namespaces  []string
}

func (f FlipperConfig) String() string {
	return fmt.Sprintf("match labels [%v] every [%s] in namespaces %v",
		f.MatchLabels, f.Interval.String(), f.Namespaces)
}

func (f *FlipperConfig) Matches(obj metav1.Object) bool {
	// match all labels
	if !matchLabels(obj.GetLabels(), f.MatchLabels) {
		return false
	}
	if !matchNamespaces(obj.GetNamespace(), f.Namespaces) {
		return false
	}
	return true
}

var defaultConfig = FlipperConfig{
	Name:        "_default_",
	MatchLabels: map[string]string{"mesh": "true"},
	Interval:    10 * time.Minute,
	Namespaces:  []string{},
}

type MatchCriteria struct {
	config  *FlipperConfig
	configs map[string]*FlipperConfig
	mu      sync.Mutex
}

func NewMatchCriteria() *MatchCriteria {
	return &MatchCriteria{
		configs: make(map[string]*FlipperConfig),
	}
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

	config := &FlipperConfig{
		Name:        flip.Name,
		MatchLabels: matchLabels,
		Interval:    flip.Spec.Interval.Duration,
		Namespaces:  flip.Spec.Match.Namespaces,
	}

	m.configs[flip.Name] = config
}

func (m *MatchCriteria) DeleteConfig(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.configs, name)
}

// returns copy of the matching config and true
// if no configs are present, it returns the defaultConfig if it matches the object
// returns zeroed flipperconfig and false, if no existing configuration match
func (m *MatchCriteria) MatchingConfig(obj metav1.Object) (FlipperConfig, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.configs) == 0 {
		if defaultConfig.Matches(obj) {
			return defaultConfig, true
		}
	}

	for _, cfg := range m.configs {
		if cfg.Matches(obj) {
			return *cfg, true
		}
	}
	return FlipperConfig{}, false
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
