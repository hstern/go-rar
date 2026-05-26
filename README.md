# go-rar

A Go library implementing
[RFC 9396 — OAuth 2.0 Rich Authorization Requests][rfc9396].

[rfc9396]: https://www.rfc-editor.org/rfc/rfc9396.html

> **Status: pre-release.** API surface is being shaped against RFC 9396
> §2–§9 with the goal of byte-stable round-trip on every spec example.
> First tag will be `v0.1.0`. The module path
> `github.com/hstern/go-rar` is stable from the first commit; do not
> depend on the repo until `v0.1.0` is tagged.

## What it is

`go-rar` is the Go ecosystem's reference implementation of the RFC 9396
`authorization_details` parameter — the discriminated-union JSON value
OAuth 2.0 uses to express fine-grained authorization.

The library handles:

- **The §2 `common` baseline** — `locations`, `actions`, `datatypes`,
  `identifier`, `privileges`. RFC 9396 deliberately defines no built-in
  `type` values, so `common` is the only carrier shipped out of the
  box; every IANA-published or consumer-defined type comes in via
  `RegisterType`.
- **JSON codec** — discriminator-driven `Unmarshal` dispatch plus
  spec-order, byte-stable `Marshal` output (`type` always first).
- **Authorization-endpoint form encoding** — `EncodeForm` /
  `DecodeForm` for the `application/x-www-form-urlencoded` case where
  the array is carried as a single URL-encoded JSON string.
- **Validation** — opt-in `Validate()` per type, returning a
  `*ValidationError` with `Rule`, `Type`, and `Reason` fields.
  `ValidateAll` joins per-element errors via `errors.Join`.
- **Forward compatibility** — unknown `type` values parse into an
  `UnknownType` carrier that preserves the wire bytes verbatim, so the
  authorization detail round-trips even when the library can't fully
  interpret it.
- **Extension** — `RegisterType(name, ctor)` lets downstream code add
  a constructor for any custom or IANA-published type. Built-in names
  are protected by `ErrTypeReserved`.

## Install

```sh
go get github.com/hstern/go-rar@latest
```

Requires Go 1.26 or newer.

## Quickstart

_To be filled in once Phase 3 (codec) lands._

## How this fits with OAuth 2.0

RFC 9396 defines the value of a single OAuth parameter
(`authorization_details`); it does not specify any new endpoints, grant
types, or HTTP flows. The same JSON shape this library encodes is
carried unchanged through three OAuth touchpoints:

- the authorization request (URL form-encoded at the authorization
  endpoint, or as a JSON member of a Pushed Authorization Request),
- the token response (as a JSON member alongside `access_token`), and
- the token introspection response (as a JSON member describing the
  authorization the access token represents).

`go-rar` is the codec for that shape; binding it into an authorization
server, resource server, or client library is the OAuth library's job.
The library plugs in alongside any stdlib `net/http` server or any
existing OAuth 2.0 client/server stack.

## Design

The library's design rationale will be summarized in `DESIGN.md` ahead
of the `v0.1.0` tag. The headline decisions:

- Sealed `AuthorizationDetail` interface with per-type concrete
  Go types — no `map[string]any`, no zero-valued union-fields struct.
- `encoding/json` stdlib with custom `UnmarshalJSON` dispatch on the
  `type` discriminator.
- **Lenient on unmarshal, strict on marshal.** Postel's law: decode
  whatever the wire gave us, validate at the marshal boundary only
  when `StrictMarshal(true)` is set.
- Byte-stable output: `type` first, then the §2 common members in
  spec order, then any type-specific members in declared order.
- Open-extension fields are `json.RawMessage`, not `map[string]any` —
  interop scenarios pin exact JSON bytes, and `map` reorders keys.

## Compatibility

- **Spec version**: `const SpecVersion = "RFC 9396"`. RFCs have no
  minor or patch numbers; errata are absorbed into Go-minor releases
  without changing the constant.
- **Go version**: 1.26+.
- **Dependencies**: standard library only, at runtime. Test
  dependencies, if any, are listed in `go.mod`.
- **Library SemVer** is independent of the spec version. Major-version
  handling follows the `go-jose` branch pattern (no versioned
  subdirectories — `vN` lives in `go.mod` on a `vN` branch).

## Contributing

Contributor conventions are in [`AGENTS.md`](AGENTS.md): commit message
style, code review expectations, the per-file SPDX header, and the
local pre-PR checks the CI also runs.

Bugs and feature ideas are welcome via the project's issue tracker.

## License

Apache-2.0. See [`LICENSE`](LICENSE) for the full text.

Every source file carries an SPDX identifier:

```go
// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0
```
