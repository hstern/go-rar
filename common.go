// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

// Common carries the RFC 9396 §2 baseline members shared by every
// authorization_details element: `locations`, `actions`, `datatypes`,
// `identifier`, `privileges`. All five are OPTIONAL per the spec.
//
// This file declares Common as an empty struct so the package
// compiles ahead of the field-populating commit. The fields land in
// the immediate follow-up commit; the empty type lets the sealed
// [AuthorizationDetail] interface — whose Common() method returns
// *Common — be declared and referenced now without forcing the two
// changes into a single PR.
//
// Consumers should treat Common as opaque until the fields land;
// constructing a zero-value Common today is harmless but carries no
// information.
type Common struct{}
