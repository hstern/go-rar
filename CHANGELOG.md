# Changelog

All notable changes to `go-rar` are documented here. The format is
loosely based on [Keep a Changelog](https://keepachangelog.com/), and
the project adheres to [Semantic Versioning](https://semver.org/).

The library's version is independent of the spec it implements
(RFC 9396). Errata to the RFC are absorbed into Go-minor releases
without bumping the spec-version constant.

## [Unreleased]

### Added

- **`Extension` embeddable base struct + `Baseline` public alias.**
  Consumer-defined types satisfying `AuthorizationDetail` from outside
  the `rar` package now embed `rar.Extension` to inherit the sealed
  `sealed()` marker, the §2 baseline fields (via the `Baseline`
  alias), and default `Type` / `Common` / `Validate` / `MarshalJSON` /
  `UnmarshalJSON` implementations. The consumer adds type-specific
  fields, overrides the methods that need type-specific behavior, and
  registers via `RegisterType`. The previously-documented
  sealed-interface limit on consumer typed extensions no longer
  applies; the README's Extensibility section has been rewritten to
  document the new pattern, and `extension_external_test.go`
  exercises the out-of-package consumer pattern end-to-end (including
  the compile-time assertion that the embedding-grants-sealed path
  holds).

### Fixed

- **Explicit empty arrays now round-trip byte-stably.** A payload
  carrying `"actions": []` (or any §2 baseline slice field as an
  explicit empty array — `locations`, `actions`, `datatypes`,
  `privileges`) re-marshals with the field preserved as `[]` rather
  than being elided. Previously, the stdlib `omitempty` rule on
  `[]string` could not distinguish nil from length-zero, so any
  explicit empty array was dropped on the marshal side. The
  hand-written `*CommonType.MarshalJSON` introduced by this fix emits
  each slice field iff non-nil: nil elides (the spec's "absent
  member" shape), and non-nil-empty emits `[]` (the present-but-empty
  shape consumers may pin on the wire). The fix is observation-only
  for the common case — payloads whose slice fields are either nil or
  non-empty produce byte-identical output to the prior implementation;
  only the explicit-empty case changes.

## [0.1.0] - 2026-05-26

Initial public release. Targets RFC 9396 — OAuth 2.0 Rich Authorization
Requests (Proposed Standard, 2023-05). Zero non-stdlib runtime
dependencies; Go 1.26+.

### Added

- **Sealed `AuthorizationDetail` interface** with `Type()` / `Common()`
  / `Validate()` plus an unexported `sealed()` marker. Confines
  implementations to the package; downstream code dispatches new types
  through `RegisterType`.
- **`Common` struct** carrying the RFC 9396 §2 baseline members
  (`locations`, `actions`, `datatypes`, `identifier`, `privileges`),
  all optional and `omitempty` on marshal.
- **`CommonType`** — the built-in `AuthorizationDetail` implementation
  registered under the `common` discriminator; the §2-only carrier for
  details that don't extend the union with type-specific members.
- **`UnknownType`** — the forward-compatibility carrier that preserves
  the wire bytes verbatim in `Raw` for any `type` value not present in
  the dispatch table. Round-trips byte-stably.
- **`AuthorizationDetails`** type alias for `[]AuthorizationDetail` so
  function signatures read naturally.
- **JSON codec**: `Parse`, `ParseArray`, plus `UnmarshalJSON` /
  `MarshalJSON` on the built-in carriers. Lenient on unmarshal (extra
  fields silently dropped per the spec's MUST-ignore-unknown
  convention); spec-order, byte-stable on marshal (`type` first, §2
  members in spec order).
- **Form codec**: `EncodeForm` / `DecodeForm` for the
  authorization-endpoint case where `authorization_details` is a form
  parameter carrying a JSON-encoded array.
- **Validation**: opt-in `Validate()` per type plus `ValidateAll`
  (joins per-element errors via `errors.Join`). Rules implemented:
  `type-required`, `locations-uri`, and `<field>-element-empty` for
  `actions` / `datatypes` / `privileges`. `*ValidationError` carries
  structured `Rule` / `Type` / `Reason` fields.
- **`SetStrictMarshal(bool)`** — opt-in fail-fast on outbound: when
  enabled, `MarshalJSON` runs `Validate()` first. Default false
  (Postel's law on outbound). Atomic-backed for safe concurrent reads.
- **`RegisterType(name, ctor)`** — registry for consumer-defined or
  IANA-published `type` values. Returns `ErrTypeReserved` on built-in
  collisions (only `common` is built-in).
- **Error umbrella** `Err` plus the `ErrTypeReserved` sentinel; every
  `*ValidationError` and library-emitted error matches
  `errors.Is(err, rar.Err)` for categorical branching.
- **`internal/specfixtures/`** embeds every example payload from
  RFC 9396 §2–§9 in compact canonical form for the conformance suite.
- **Conformance tests** assert byte-stable round-trip on every spec
  example through both the JSON and form codec paths.
- **Forward-compat and extension tests** exercise the `*UnknownType`
  fallback and the `RegisterType` end-to-end flow.

### Known limitations

- **Empty arrays cannot survive a marshal cycle.** Stdlib `omitempty`
  on `[]string` treats both `nil` and length-zero as absent and elides
  the field, so a payload that explicitly carries `"actions": []`
  re-marshals to a payload without the field. RFC 9396's published
  examples never carry explicit empty arrays, so the spec-fixture
  round-trip is unaffected; downstream consumers needing exact
  empty-array preservation should track or open a follow-up.
- **The `AuthorizationDetail` interface is sealed to the package.**
  Consumers cannot supply their own concrete types from outside the
  package; `RegisterType` is most useful today for registering
  alternative constructors that return one of the built-in carriers.
  Richer downstream typing is a known limitation tracked for post-
  `v0.1.0`. Consumers needing typed access to extension fields parse
  `*UnknownType.Raw` themselves.

[Unreleased]: https://github.com/hstern/go-rar/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/hstern/go-rar/releases/tag/v0.1.0
