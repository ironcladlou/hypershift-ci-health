package jobregistry

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadFile reads and validates a generated JSON registry.
func LoadFile(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read job registry %q: %w", path, err)
	}
	registry, err := Load(data)
	if err != nil {
		return nil, fmt.Errorf("load job registry %q: %w", path, err)
	}
	return registry, nil
}

// Load decodes and validates a generated JSON registry.
func Load(data []byte) (*Registry, error) {
	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	if err := registry.Validate(); err != nil {
		return nil, err
	}
	return &registry, nil
}

// Validate checks the registry contract required by consumers.
func (r *Registry) Validate() error {
	if r.APIVersion != currentAPIVersion {
		return fmt.Errorf("unsupported job registry API version %q", r.APIVersion)
	}
	seen := make(map[string]struct{}, len(r.Jobs))
	for i := range r.Jobs {
		job := &r.Jobs[i]
		if job.ID == "" || job.Name == "" {
			return fmt.Errorf("job at index %d has an empty ID or name", i)
		}
		if _, found := seen[job.ID]; found {
			return fmt.Errorf("duplicate job ID %q", job.ID)
		}
		seen[job.ID] = struct{}{}
		switch job.Type {
		case "presubmit":
			if job.Presubmit == nil {
				return fmt.Errorf("presubmit job %q has no presubmit configuration", job.ID)
			}
		case "periodic":
			if job.Presubmit != nil {
				return fmt.Errorf("periodic job %q has presubmit configuration", job.ID)
			}
		default:
			return fmt.Errorf("job %q has unsupported type %q", job.ID, job.Type)
		}
	}
	if err := validatePresubmitPeriodicRelationships(r, r.Index()); err != nil {
		return fmt.Errorf("invalid presubmit-periodic relationships: %w", err)
	}
	return nil
}

// Index returns the registry jobs keyed by stable ID.
func (r *Registry) Index() map[string]*Job {
	result := make(map[string]*Job, len(r.Jobs))
	for i := range r.Jobs {
		result[r.Jobs[i].ID] = &r.Jobs[i]
	}
	return result
}
