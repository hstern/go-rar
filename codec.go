// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Parse decodes a single authorization_details element into the
// concrete [AuthorizationDetail] the registry dispatches to from the
// wire `type` discriminator.
//
// The raw bytes MUST be a JSON object (RFC 9396 §2). The object's
// `type` member selects the concrete carrier:
//
//   - If the registry has a constructor for the `type` value (either
//     a built-in or a consumer registration installed via
//     [RegisterType]), Parse calls it and delegates the field decode
//     to the returned value's UnmarshalJSON. The returned interface
//     wraps that concrete type.
//
//   - If no constructor matches, Parse returns an [*UnknownType]
//     carrying the discriminator and the entire JSON object verbatim
//     in its Raw field. This is the forward-compatibility path the
//     library's sealed-interface posture relies on (see [UnknownType]
//     for the rationale) — unknown types are NOT an error.
//
//   - If the JSON does not parse, Parse returns a wrapped stdlib
//     [encoding/json] error matching errors.Is(err, [Err]).
//
//   - If the `type` member is missing or empty, Parse returns a
//     [*ValidationError] with Rule "type-required" (which also
//     matches errors.Is(err, [Err])).
//
// Parse is the discriminator-aware entry point for a single element;
// callers parsing the wire array should use [ParseArray] instead.
func Parse(raw json.RawMessage) (AuthorizationDetail, error) {
	// Peek the discriminator without committing to a typed struct.
	// A minimal probe is cheap and avoids double-decoding the typed
	// fields — the registry-selected ctor's UnmarshalJSON does that
	// once below on the original bytes.
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("%w: parse authorization_detail: %w", Err, err)
	}
	if probe.Type == "" {
		return nil, &ValidationError{
			Rule:   "type-required",
			Reason: "type member missing or empty",
		}
	}

	// Default to the forward-compat carrier; the registry hit
	// overrides when the discriminator is known.
	var dst AuthorizationDetail = &UnknownType{}
	if ctor := lookup(probe.Type); ctor != nil {
		dst = ctor()
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return nil, fmt.Errorf("%w: parse %q: %w", Err, probe.Type, err)
	}
	return dst, nil
}

// ParseArray decodes the JSON array form of the authorization_details
// parameter (RFC 9396 §2). Each element is dispatched through [Parse]
// in order.
//
// Per-element errors halt the parse — ParseArray does not partial-
// return. The first element that fails is identified by its zero-based
// index in the wrapping error chain; the underlying [*ValidationError]
// (or wrapped stdlib JSON error) is reachable with errors.As.
//
// A successful parse returns an [AuthorizationDetails] slice of the
// same length as the input array, with each entry holding the concrete
// carrier the registry selected (or an [*UnknownType] for unrecognized
// discriminators).
func ParseArray(raw json.RawMessage) (AuthorizationDetails, error) {
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil, fmt.Errorf("%w: parse authorization_details array: %w", Err, err)
	}
	out := make(AuthorizationDetails, 0, len(elems))
	for i, e := range elems {
		d, err := Parse(e)
		if err != nil {
			return nil, fmt.Errorf("element %d: %w", i, err)
		}
		out = append(out, d)
	}
	return out, nil
}

// UnmarshalJSON populates the receiver from the wire object: the
// `type` discriminator lands in TypeName, and the RFC 9396 §2 baseline
// members land in the embedded [Common]. Members outside that set are
// silently dropped — the lenient-unmarshal posture (Postel's law)
// means a payload with extra fields parses cleanly, leaving any
// strict-shape enforcement to the opt-in [CommonType.Validate].
//
// UnmarshalJSON does NOT enforce the "type non-empty" rule. That check
// lives in [Parse] (which is called before dispatch reaches here) and
// in the validation phase via the "type-required" rule on
// [ValidationError]; UnmarshalJSON's job is to populate fields from
// whatever bytes the wire produced, not to gate on them.
//
// The aux struct mirrors CommonType's wire shape — `type` plus the
// five §2 baseline members — and is the standard pattern for
// implementing UnmarshalJSON on a type that embeds another struct
// without triggering infinite recursion (calling json.Unmarshal on a
// *CommonType from inside *CommonType.UnmarshalJSON would re-enter
// this method).
func (c *CommonType) UnmarshalJSON(b []byte) error {
	var aux struct {
		Type       string   `json:"type"`
		Locations  []string `json:"locations,omitempty"`
		Actions    []string `json:"actions,omitempty"`
		Datatypes  []string `json:"datatypes,omitempty"`
		Identifier string   `json:"identifier,omitempty"`
		Privileges []string `json:"privileges,omitempty"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	c.TypeName = aux.Type
	c.Locations = aux.Locations
	c.Actions = aux.Actions
	c.Datatypes = aux.Datatypes
	c.Identifier = aux.Identifier
	c.Privileges = aux.Privileges
	return nil
}

// MarshalJSON serializes the receiver as a JSON object whose member
// order matches every example in RFC 9396 §2–§9: the `type`
// discriminator first, then the five §2 baseline members in the order
// the spec enumerates them — locations, actions, datatypes,
// identifier, privileges. RFC 9396 does not formally mandate field
// ordering on the wire, but every published example puts `type` first
// and lists the §2 baseline in the spec's enumeration order, so the
// library produces output that is byte-identical to those examples
// (modulo whitespace). Byte-stability matters because consumers diff
// produced payloads against the spec's figures and against captures
// from third-party implementations during interop work.
//
// The implementation walks the §2 baseline fields in spec order and
// emits each field conditionally into a single buffer. Hand-buffering
// (rather than delegating to the stdlib through an anonymous struct
// with `omitempty` tags) is required to preserve the nil-vs-length-
// zero distinction on slice fields — see the empty-array section
// below. The per-field write path delegates to [encoding/json.Marshal]
// for the value itself, so JSON-escaping of the string and slice
// contents stays in stdlib hands; only the member-ordering and
// elision decisions live in this method.
//
// Strict-marshal opt-in. When the package-wide toggle is enabled via
// [SetStrictMarshal] (off by default), MarshalJSON runs
// [CommonType.Validate] before serializing and returns the
// [*ValidationError] verbatim in place of writing potentially-
// malformed JSON. The error propagates through every stdlib path that
// invokes MarshalJSON — [encoding/json.Marshal] surfaces the inner
// error to the caller, and [EncodeForm] (which delegates to
// `json.Marshal` on the slice) likewise propagates it. With the
// toggle off, MarshalJSON is purely lenient: a CommonType with an
// empty TypeName still marshals, producing output that is technically
// out-of-spec (RFC 9396 §2 requires `type` to be non-empty) but
// faithful to whatever fields the receiver carries. The lenient
// default matches the library's Postel's-law posture on outbound —
// producers usually know what they are producing — and avoids the
// per-marshal Validate cost for the common case where the consumer
// has its own validation pipeline upstream. See [SetStrictMarshal]
// for the toggle's concurrency contract and the test-isolation
// pattern.
//
// Empty-array carriage. The four baseline slice fields
// ([Common.Locations], [Common.Actions], [Common.Datatypes],
// [Common.Privileges]) distinguish nil from length-zero on output:
// a nil slice is elided entirely (matching the spec's "absent member"
// shape), and a non-nil length-zero slice is emitted as the JSON
// literal `[]`. A payload that arrives as
// `{"type":"common","locations":[]}` therefore round-trips byte-
// stably — Parse populates Locations as a non-nil empty slice (sub-
// decision §3 in the design notes preserves the distinction on
// unmarshal), and MarshalJSON now preserves it on the wire. The
// stdlib `omitempty` rule on `[]string` cannot make this distinction
// because it collapses both nil and length-zero into "empty" and
// elides the field; the hand-written marshal path here is the
// targeted fix. RFC 9396's published examples never use an explicit
// empty array, so the spec-fixture round-trip is unchanged by the
// fix; the change matters only for downstream consumers that observe
// the explicit-empty case on the wire.
//
// [Common.Identifier] is a string, not a slice, so the nil-vs-empty
// distinction does not apply: an empty string elides the member,
// matching the stdlib `omitempty` semantics the field used to carry.
// Go's string type cannot represent "present-but-empty" the way a
// pointer-or-slice can; the carriage of an explicit-empty-string
// identifier is left to a future API revision if a real-world payload
// ever requires it.
func (c *CommonType) MarshalJSON() ([]byte, error) {
	if strictMarshal() {
		if err := c.Validate(); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	buf.WriteByte('{')

	// type — required, always first. Routed through json.Marshal so
	// the string contents are escaped by stdlib rules (quotes,
	// control characters, non-ASCII). MarshalJSON does not enforce
	// non-emptiness here — that's the strict-marshal Validate path
	// above and the Parse-time check on inbound.
	buf.WriteString(`"type":`)
	typeBytes, err := json.Marshal(c.TypeName)
	if err != nil {
		return nil, fmt.Errorf("marshal type: %w", err)
	}
	buf.Write(typeBytes)

	// §2 baseline slice fields, in spec order. Each is emitted iff
	// non-nil; length-zero non-nil slices marshal as `[]`. See the
	// "Empty-array carriage" godoc above.
	if c.Locations != nil {
		buf.WriteString(`,"locations":`)
		if err := writeStringSliceJSON(&buf, c.Locations); err != nil {
			return nil, fmt.Errorf("marshal locations: %w", err)
		}
	}
	if c.Actions != nil {
		buf.WriteString(`,"actions":`)
		if err := writeStringSliceJSON(&buf, c.Actions); err != nil {
			return nil, fmt.Errorf("marshal actions: %w", err)
		}
	}
	if c.Datatypes != nil {
		buf.WriteString(`,"datatypes":`)
		if err := writeStringSliceJSON(&buf, c.Datatypes); err != nil {
			return nil, fmt.Errorf("marshal datatypes: %w", err)
		}
	}
	// identifier — string field, keep stdlib omitempty semantics
	// (empty string elides). See the godoc above for why a string
	// cannot carry the same nil-vs-empty distinction a slice can.
	if c.Identifier != "" {
		buf.WriteString(`,"identifier":`)
		idBytes, err := json.Marshal(c.Identifier)
		if err != nil {
			return nil, fmt.Errorf("marshal identifier: %w", err)
		}
		buf.Write(idBytes)
	}
	if c.Privileges != nil {
		buf.WriteString(`,"privileges":`)
		if err := writeStringSliceJSON(&buf, c.Privileges); err != nil {
			return nil, fmt.Errorf("marshal privileges: %w", err)
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// writeStringSliceJSON writes a `[]string` as a JSON array into buf
// using stdlib encoding rules. The stdlib emits a non-nil length-zero
// slice as `[]` (a nil slice would also encode as `[]` here, but
// MarshalJSON only invokes this helper after the nil check above —
// the elide-on-nil decision is the caller's, not the helper's). The
// helper is a thin wrapper rather than a one-liner so the per-field
// emit sites in MarshalJSON read uniformly.
func writeStringSliceJSON(buf *bytes.Buffer, ss []string) error {
	b, err := json.Marshal(ss)
	if err != nil {
		return err
	}
	buf.Write(b)
	return nil
}

// MarshalJSON emits the captured JSON object verbatim. For an
// UnknownType produced by [Parse], Raw holds the entire original
// object bytes (see [UnknownType.UnmarshalJSON]), so the output is
// byte-identical to the input — the round-trip guarantee the
// forward-compat carrier exists to provide. The bytes are returned
// as-is; stdlib [encoding/json] accepts a [json.RawMessage] return
// without re-parsing.
//
// When Raw is empty — the case where a consumer hand-constructs an
// UnknownType (TypeName set, Raw never populated) to forward an
// element through a producer surface — MarshalJSON synthesizes a
// minimal `{"type":"<TypeName>"}` object from TypeName. This is the
// most useful of the three options (synthesize, emit `{}`, return an
// error): the synthesized output is well-formed JSON satisfying the
// spec's MUST on the `type` member, and a hand-built UnknownType
// remains marshalable. An empty TypeName in this branch produces
// `{"type":""}`, which is intentionally still well-formed JSON
// (validation of the `type` value is the consumer's job, not
// MarshalJSON's).
//
// Note that the synthesized branch necessarily does NOT round-trip:
// re-parsing `{"type":"x"}` produces an UnknownType whose Raw is the
// 12-byte object, not the empty slice the synthesizer started from.
// Consumers that care about round-trip identity should populate Raw
// directly (typically by constructing the carrier via [Parse]).
func (u *UnknownType) MarshalJSON() ([]byte, error) {
	if len(u.Raw) == 0 {
		// Synthesize the minimal `{"type":"<TypeName>"}` object using
		// the stdlib encoder. Building the JSON by hand would require
		// escaping the type string (quotes, control chars, etc.); the
		// encoder already does that correctly.
		return json.Marshal(struct {
			Type string `json:"type"`
		}{Type: u.TypeName})
	}
	return u.Raw, nil
}

// UnmarshalJSON captures the entire JSON object verbatim into Raw and
// surfaces the `type` discriminator on TypeName. The verbatim capture
// is what makes [UnknownType] safe to round-trip — the inverse
// [UnknownType.MarshalJSON] emits Raw byte-for-byte, so an unrecognized
// type flows through the library without reordering keys, dropping
// members, or normalizing whitespace.
//
// The input slice is copied before storing in Raw because the stdlib
// JSON decoder may reuse its input buffer; aliasing it directly would
// expose the receiver to corruption when the caller reuses the
// buffer.
func (u *UnknownType) UnmarshalJSON(b []byte) error {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return err
	}
	u.TypeName = probe.Type
	raw := make(json.RawMessage, len(b))
	copy(raw, b)
	u.Raw = raw
	return nil
}
