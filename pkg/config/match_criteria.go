package config

import (
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/api/v1alpha1"
)

type MatchCriteria struct {
	config *FlipperConfig
	mu     sync.Mutex
}

type FlipperConfig struct {
	MatchLabels map[string]string
	// MatchValue string
	Interval   time.Duration
	Namespaces []string
}

func (f FlipperConfig) String() string {
	return fmt.Sprintf("match labels [%v] every [%s] in namespaces %v",
		f.MatchLabels, f.Interval.String(), f.Namespaces)
}

var defaultConfig = FlipperConfig{
	// MatchLabel: "mesh",
	// MatchValue: "true",
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

// TODO should we validate here or in the flipper API itself.
func (m *MatchCriteria) Validate(flip *flipperiov1alpha1.Flipper) error {
	return nil
}

func (m *MatchCriteria) UpdateConfig(flip *flipperiov1alpha1.Flipper) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// // read only one value
	// var matchLabel, matchValue string
	// for k, v := range flip.Spec.Match.Labels {
	// 	matchLabel, matchValue = k, v
	// 	break
	// }

	matchLabels := make(map[string]string)
	for k, v := range flip.Spec.Match.Labels {
		matchLabels[k] = v
	}

	m.config = &FlipperConfig{
		// MatchLabel: matchLabel,
		// MatchValue: matchValue,
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
	// for k, v := range cfg.MatchLabels {
	// 	if obj.GetLabels()[]
	// }
	// if obj.GetLabels()[cfg.MatchLabel] != cfg.MatchValue {
	// 	return false
	// }
	// empty means we match all namespaces
	// if len(cfg.Namespaces) == 0 {
	// 	return true
	// }
	// for _, ns := range cfg.Namespaces {
	// 	if ns == obj.GetNamespace() {
	// 		return true
	// 	}
	// }
	if !matchNamespaces(obj.GetNamespace(), cfg.Namespaces) {
		return false
	}
	return true
}

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

func matchLabels(act, exp map[string]string) bool {
	// if no expected labels, then it matches everything.
	if len(exp) == 0 {
		return true
	}
	if len(act) == 0 {
		return false
	}
	for k, v := range exp {
		if act[k] != v {
			return false
		}
	}

	return true
}
