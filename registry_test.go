// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

// resetRegistryForTest restores the dispatch table to its built-in
// state and schedules the same restoration via t.Cleanup so each test
// starts from a known baseline and leaves no consumer-registered
// entries behind for the next test. Tests that mutate the global
// registry (by calling [RegisterType] for any non-built-in name) MUST
// call this helper.
func resetRegistryForTest(t *testing.T) {
	t.Helper()
	typeRegistry.mu.Lock()
	typeRegistry.m = builtinTypes()
	typeRegistry.mu.Unlock()
	t.Cleanup(func() {
		typeRegistry.mu.Lock()
		typeRegistry.m = builtinTypes()
		typeRegistry.mu.Unlock()
	})
}

func TestRegisterType_ReservedName(t *testing.T) {
	resetRegistryForTest(t)

	tests := []struct {
		name     string
		typeName string
	}{
		{name: "common is reserved", typeName: "common"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := RegisterType(tc.typeName, func() AuthorizationDetail { return &CommonType{} })
			if err == nil {
				t.Fatalf("RegisterType(%q) = nil error; want ErrTypeReserved", tc.typeName)
			}
			if !errors.Is(err, ErrTypeReserved) {
				t.Errorf("errors.Is(err, ErrTypeReserved) = false; want true (err=%v)", err)
			}
			if !errors.Is(err, Err) {
				t.Errorf("errors.Is(err, Err) = false; want true (err=%v)", err)
			}
		})
	}
}

func TestRegisterType_InstallsConstructor(t *testing.T) {
	resetRegistryForTest(t)

	want := func() AuthorizationDetail { return &CommonType{TypeName: "payment_initiation"} }
	if err := RegisterType("payment_initiation", want); err != nil {
		t.Fatalf("RegisterType(\"payment_initiation\", _) = %v; want nil", err)
	}

	got := lookup("payment_initiation")
	if got == nil {
		t.Fatalf("lookup(\"payment_initiation\") = nil; want the registered constructor")
	}
	// Function values are not == comparable in Go; compare the
	// underlying code pointer via reflect.
	if reflect.ValueOf(got).Pointer() != reflect.ValueOf(want).Pointer() {
		t.Errorf("lookup returned a different function pointer than the one registered")
	}

	// And calling it produces the value the constructor returns.
	val := got()
	cv, ok := val.(*CommonType)
	if !ok {
		t.Fatalf("registered constructor returned %T; want *CommonType", val)
	}
	if cv.TypeName != "payment_initiation" {
		t.Errorf("constructor produced TypeName=%q; want %q", cv.TypeName, "payment_initiation")
	}
}

func TestRegisterType_ReplacesPreviousRegistration(t *testing.T) {
	resetRegistryForTest(t)

	first := func() AuthorizationDetail { return &CommonType{TypeName: "first"} }
	second := func() AuthorizationDetail { return &CommonType{TypeName: "second"} }

	if err := RegisterType("custom_type", first); err != nil {
		t.Fatalf("RegisterType(\"custom_type\", first) = %v; want nil", err)
	}
	if err := RegisterType("custom_type", second); err != nil {
		t.Fatalf("RegisterType(\"custom_type\", second) = %v; want nil", err)
	}

	got := lookup("custom_type")
	if got == nil {
		t.Fatalf("lookup(\"custom_type\") = nil after re-registration")
	}
	if reflect.ValueOf(got).Pointer() != reflect.ValueOf(second).Pointer() {
		t.Errorf("re-registration did not replace the prior constructor")
	}
}

func TestLookup_Unregistered(t *testing.T) {
	resetRegistryForTest(t)

	if got := lookup("unregistered_type_xyz"); got != nil {
		t.Errorf("lookup(\"unregistered_type_xyz\") returned a non-nil constructor; want nil")
	}
}

func TestLookup_BuiltinCommon(t *testing.T) {
	resetRegistryForTest(t)

	ctor := lookup("common")
	if ctor == nil {
		t.Fatalf("lookup(\"common\") = nil; want the built-in constructor")
	}
	val := ctor()
	if val == nil {
		t.Fatalf("built-in \"common\" constructor returned a nil AuthorizationDetail")
	}
	cv, ok := val.(*CommonType)
	if !ok {
		t.Fatalf("built-in \"common\" constructor returned %T; want *CommonType", val)
	}
	if cv == nil {
		t.Fatalf("built-in \"common\" constructor returned a typed-nil *CommonType")
	}
	// And the value satisfies AuthorizationDetail — verified by the
	// type assertion above plus the package-level interface assertion
	// in common_type.go, but a compile-time check here pins it for
	// this test surface too.
	var _ AuthorizationDetail = cv
}

func TestRegistry_ConcurrentRegisterAndLookup(t *testing.T) {
	resetRegistryForTest(t)

	// Pre-populate one name so concurrent lookups have at least one
	// non-nil target to fetch alongside the racing registrations.
	if err := RegisterType("seed", func() AuthorizationDetail { return &CommonType{TypeName: "seed"} }); err != nil {
		t.Fatalf("seed RegisterType = %v; want nil", err)
	}

	const goroutines = 10
	var wg sync.WaitGroup

	for i := range goroutines {
		wg.Go(func() {
			name := "concurrent_" + string(rune('a'+i))
			if err := RegisterType(name, func() AuthorizationDetail { return &CommonType{TypeName: name} }); err != nil {
				t.Errorf("RegisterType(%q) = %v; want nil", name, err)
			}
		})
		wg.Go(func() {
			// The specific lookup target doesn't matter — the race
			// detector only cares that the read happens in parallel
			// with the writes above.
			_ = lookup("seed")
			_ = lookup("common")
		})
	}

	wg.Wait()
}
