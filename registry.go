// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import "sync"

// typeRegistry holds the dispatch table keyed by the wire `type`
// discriminator. Reads dominate (UnmarshalJSON looks up the
// constructor on every parse); writes are rare (RegisterType is
// expected to run at init() time in consumers adding new
// IANA-registered types). sync.RWMutex is the right shape for that
// read-heavy access pattern.
//
// The map is wrapped in an anonymous struct so the mutex and the map
// it guards travel together — accidentally reading or writing the
// map without taking the lock requires going out of one's way.
var typeRegistry = struct {
	m  map[string]func() AuthorizationDetail
	mu sync.RWMutex
}{m: builtinTypes()}

// builtinTypes returns a freshly-allocated copy of the dispatch
// entries the library ships out of the box. Returning a fresh map on
// every call means [RegisterType] can compare against it for the
// reserved-name check without risk that a caller mutates the result
// and changes what counts as "built-in", and [resetRegistryForTest]
// can reinstall a clean slate without sharing aliased state with the
// previous test.
//
// RFC 9396 deliberately defines no built-in type values; the single
// entry "common" maps to a fresh *CommonType (the §2-only carrier),
// so any payload with type:"common" parses into a *CommonType. Every
// other type value either dispatches to a consumer-registered
// constructor (installed via [RegisterType]) or to *UnknownType, the
// forward-compatibility carrier.
func builtinTypes() map[string]func() AuthorizationDetail {
	return map[string]func() AuthorizationDetail{
		"common": func() AuthorizationDetail { return &CommonType{} },
	}
}

// RegisterType installs a constructor for a new authorization-detail
// type. ctor must return a freshly-zeroed value satisfying
// [AuthorizationDetail]; the codec calls it once per UnmarshalJSON
// dispatch and then delegates to the returned value's UnmarshalJSON.
//
// Names returned by [builtinTypes] are reserved — registering
// "common" returns [ErrTypeReserved] (which wraps [Err]). All other
// names are open; re-registering a previously consumer-registered
// name silently replaces the prior constructor. The replace-on-
// rewrite behavior is convenient for tests that swap a constructor
// in and out; init-time consumers should register each name at most
// once per process.
//
// Concurrency: safe to call concurrently with parse operations. The
// intended use is from init() functions, where contention is
// effectively absent; the RWMutex covers the rare runtime case
// (e.g. a plugin framework registering at hot-reload time).
func RegisterType(name string, ctor func() AuthorizationDetail) error {
	if _, ok := builtinTypes()[name]; ok {
		return ErrTypeReserved
	}
	typeRegistry.mu.Lock()
	defer typeRegistry.mu.Unlock()
	typeRegistry.m[name] = ctor
	return nil
}

// lookup returns the constructor registered for name, or nil if no
// entry exists. It is unexported because it is a codec internal —
// callers outside this package reach the registry through [Parse] and
// the per-type UnmarshalJSON methods, not directly.
func lookup(name string) func() AuthorizationDetail {
	typeRegistry.mu.RLock()
	defer typeRegistry.mu.RUnlock()
	return typeRegistry.m[name]
}
