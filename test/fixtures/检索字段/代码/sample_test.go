// SPDX-License-Identifier: MIT
// SEARCH_GO_TEST_2026 identifies the Go test-file fixture.
package searchfixture

import (
	"strings"
	"testing"
)

func TestFeatureMarker(t *testing.T) {
	t.Helper()
	const marker = "SEARCH_GO_TEST_2026"
	if strings.TrimSpace(marker) != marker {
		t.Fatalf("marker contains surrounding whitespace: %q", marker)
	}
}
