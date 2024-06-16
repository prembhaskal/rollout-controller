package config

import (
	"fmt"
	"sync"
	"time"

	flipperiov1alpha1 "github.com/prembhaskal/rollout-controller/api/v1alpha1"
)

type MatchCriteria struct {
	config *FlipperConfig
	mu     sync.Mutex
}

type FlipperConfig struct {
	MatchLabel string
	MatchValue string
	Interval   time.Duration
	Namespace  string
}

func (f FlipperConfig) String() string {
	return fmt.Sprintf("match label [%s:%s] every [%s]s in namespace %s",
		f.MatchLabel, f.MatchValue, f.Interval.String(), f.Namespace)
}

//	var defaultSpec = flipperiov1alpha1.FlipperSpec{
//		Interval: v1.Duration{Duration: 10 * time.Minute},
//		Match: flipperiov1alpha1.MatchSpec{
//			Labels:    map[string]string{"mesh": "true"},
//			Namespace: "",
//		},
//	}
var defaultConfig = FlipperConfig{
	MatchLabel: "mesh",
	MatchValue: "true",
	Interval:   5 * time.Minute,
	Namespace:  "",
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

	// read only one value
	var matchLabel, matchValue string
	for k, v := range flip.Spec.Match.Labels {
		matchLabel, matchValue = k, v
		break
	}

	m.config = &FlipperConfig{
		MatchLabel: matchLabel,
		MatchValue: matchValue,
		Interval:   flip.Spec.Interval.Duration,
		Namespace:  flip.Spec.Match.Namespace,
	}

}

func (m *MatchCriteria) DeleteConfig() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = nil
}
