// SPDX-License-Identifier: MIT
// Package searchfixture contains compact code-search samples.
package searchfixture

import (
	"fmt"
	environment "os"
	"strings"
)

const FeatureMarker = "SEARCH_GO_FEATURE_2026"

// FeatureSummary is an exported top-level declaration.
type FeatureSummary struct {
	Label string
	Ready bool
}

var DefaultFeatureSummary = FeatureSummary{Label: "feature"}

// BuildSummary normalizes a label and records whether the feature is enabled.
func BuildSummary(raw string) (FeatureSummary, error) {
	// TODO: make the empty-label error include the caller-provided value.
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return FeatureSummary{}, fmt.Errorf("empty feature label")
	}
	// FIXME: treat whitespace-only environment values as disabled.
	ready := environment.Getenv("SEARCH_GO_FEATURE_2026") != ""
	return FeatureSummary{Label: trimmed, Ready: ready}, nil
}

// EnvironmentValue demonstrates an aliased os.Getenv call.
func EnvironmentValue() string {
	return environment.Getenv("SEARCH_GO_FEATURE_2026")
}
