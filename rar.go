// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

// Package rar implements RFC 9396 Rich Authorization Requests —
// the typed encoder/decoder/validator for the OAuth 2.0
// authorization_details parameter.
//
// The package is in pre-release scaffolding state; the public surface
// will be filled in as subsequent phases land (sealed interface, §2
// baseline carrier, JSON and form codec, validation, conformance
// fixtures). The only stable identifier at this point is
// [SpecVersion], the RFC the package targets.
package rar

// SpecVersion identifies the RFC this package implements. RFCs have no
// minor or patch numbers; errata to RFC 9396 are absorbed into
// Go-minor releases of this module without changing the value of this
// constant.
const SpecVersion = "RFC 9396"
