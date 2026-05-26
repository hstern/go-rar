// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package specfixtures

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestAllFixturesAreValidCompactJSON pins the "fixtures are stored
// compact" invariant. Phase-5 conformance tests (RAR-21..23) compare
// fixture bytes directly against MarshalJSON output via bytes.Equal;
// if a future contributor pretty-prints a fixture by hand, that
// comparison breaks subtly. This test catches the regression at the
// fixture layer instead.
func TestAllFixturesAreValidCompactJSON(t *testing.T) {
	for name, fx := range All() {
		t.Run(name, func(t *testing.T) {
			if !json.Valid(fx) {
				t.Fatalf("fixture is not valid JSON")
			}
			var buf bytes.Buffer
			if err := json.Compact(&buf, fx); err != nil {
				t.Fatalf("json.Compact: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), fx) {
				t.Errorf("fixture is not in compact canonical form\nstored:  %q\ncompact: %q", fx, buf.Bytes())
			}
		})
	}
}

// TestAllReturnsEveryFixture guards against a fixture being added at
// the package level but forgotten in All(). The map is the iteration
// surface for downstream tests; an omission silently shrinks their
// coverage.
func TestAllReturnsEveryFixture(t *testing.T) {
	got := All()
	want := map[string][]byte{
		"baseline":       Baseline,
		"multiple":       Multiple,
		"token_response": TokenResponse,
		"introspection":  Introspection,
	}
	if len(got) != len(want) {
		t.Fatalf("All() returned %d entries, want %d", len(got), len(want))
	}
	for name, wantBytes := range want {
		gotBytes, ok := got[name]
		if !ok {
			t.Errorf("All() missing %q", name)
			continue
		}
		if !bytes.Equal(gotBytes, wantBytes) {
			t.Errorf("All()[%q] does not match package-level variable", name)
		}
	}
}
