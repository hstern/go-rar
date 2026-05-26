// Copyright 2026 The go-rar Authors
// SPDX-License-Identifier: Apache-2.0

package rar_test

import (
	"encoding/json"
	"fmt"

	"github.com/hstern/go-rar"
)

// Example_quickstart parses a single-element authorization_details
// array, opts in to RFC 9396 §2 validation, and reports the
// discriminator of the first element. This is the same end-to-end
// shape consumers will reach for first: ParseArray to decode, then
// ValidateAll to check well-formedness.
func Example_quickstart() {
	const payload = `[{"type":"common","actions":["read","write"],"locations":["https://api.example.com/v1/data"]}]`

	details, err := rar.ParseArray([]byte(payload))
	if err != nil {
		panic(err)
	}
	if err := rar.ValidateAll(details); err != nil {
		panic(err)
	}
	fmt.Printf("parsed %d detail(s); first type = %q\n", len(details), details[0].Type())
	// Output:
	// parsed 1 detail(s); first type = "common"
}

// ExampleParse decodes a single authorization_details element. The
// returned [rar.AuthorizationDetail] wraps the concrete carrier the
// registry dispatched to (here, [*rar.CommonType] for the built-in
// "common" discriminator).
func ExampleParse() {
	d, err := rar.Parse([]byte(`{"type":"common","actions":["read"]}`))
	if err != nil {
		panic(err)
	}
	fmt.Println(d.Type())
	// Output: common
}

// ExampleParseArray decodes the JSON-array wire shape of the
// authorization_details parameter. Each element is dispatched through
// the type registry in order; unrecognized discriminators land in
// [*rar.UnknownType] rather than failing the parse.
func ExampleParseArray() {
	details, err := rar.ParseArray([]byte(`[{"type":"common"},{"type":"customer_information","actions":["read"]}]`))
	if err != nil {
		panic(err)
	}
	fmt.Printf("%d details\n", len(details))
	for _, d := range details {
		fmt.Printf("- %s\n", d.Type())
	}
	// Output:
	// 2 details
	// - common
	// - customer_information
}

// ExampleCommonType builds a [rar.CommonType] by hand and marshals it
// to the RFC 9396 §2 wire shape. The struct embeds the §2 baseline, so
// Locations, Actions, Datatypes, Identifier, and Privileges are set via
// the promoted selectors on the [rar.Common] baseline. Marshal output
// places the `type` discriminator first, followed by the §2 baseline
// members in the spec's enumeration order.
func ExampleCommonType() {
	c := &rar.CommonType{TypeName: "common"}
	c.Locations = []string{"https://api.example.com"}
	c.Actions = []string{"read"}

	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
	// Output: {"type":"common","locations":["https://api.example.com"],"actions":["read"]}
}

// ExampleUnknownType_roundTrip shows that an authorization_details
// element whose `type` discriminator is not in the registry round-trips
// byte-for-byte through [*rar.UnknownType]. This is RFC 9396's
// open-extension model in practice: a consumer built against today's
// registry stays compatible with tomorrow's IANA-registered types
// without code changes — the unknown payload flows through unchanged.
func ExampleUnknownType_roundTrip() {
	const payload = `{"type":"org.example.future","customField":42}`
	d, err := rar.Parse([]byte(payload))
	if err != nil {
		panic(err)
	}
	u := d.(*rar.UnknownType)
	fmt.Println(u.TypeName)
	b, err := json.Marshal(u)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
	// Output:
	// org.example.future
	// {"type":"org.example.future","customField":42}
}

// ExampleRegisterType installs a constructor for an additional
// authorization_details discriminator. The library's
// [rar.AuthorizationDetail] interface is sealed, so consumer
// registrations return one of the built-in carriers ([*rar.CommonType]
// or [*rar.UnknownType]); the registration changes which carrier the
// codec dispatches to for that discriminator.
//
// This example deliberately omits a `// Output:` line. Examples share
// the package's global state with regular tests, and a `// Output:`
// line would mark this example for execution by `go test`, mutating
// the process-wide dispatch table in a way that other tests could
// observe under -shuffle. The example still renders inline in godoc;
// consumers reading it get the usage shape without the side effect.
func ExampleRegisterType() {
	// Register an alternative carrier for a namespaced discriminator.
	// In production, register at init() time and pick a name that will
	// not collide with other registrations in the process.
	if err := rar.RegisterType("example.com:registered", func() rar.AuthorizationDetail {
		return &rar.CommonType{}
	}); err != nil {
		panic(err)
	}

	d, err := rar.Parse([]byte(`{"type":"example.com:registered","actions":["read"]}`))
	if err != nil {
		panic(err)
	}
	// d is now *CommonType (instead of falling through to *UnknownType).
	_, ok := d.(*rar.CommonType)
	fmt.Println(ok)
}

// ExampleEncodeForm produces the value string for the
// `authorization_details` form parameter carried on the OAuth 2.0
// authorization endpoint. The returned string is the JSON-encoded
// array; the URL-encoding step belongs to the caller's form encoder
// (typically [net/url.Values.Encode]).
func ExampleEncodeForm() {
	c := &rar.CommonType{TypeName: "common"}
	c.Actions = []string{"read"}
	value, err := rar.EncodeForm(rar.AuthorizationDetails{c})
	if err != nil {
		panic(err)
	}
	fmt.Println(value)
	// Output: [{"type":"common","actions":["read"]}]
}

// ExampleDecodeForm parses the value of the `authorization_details`
// form parameter — the JSON-encoded array after the form decoder has
// URL-decoded it. It is the inverse of [rar.EncodeForm].
func ExampleDecodeForm() {
	value := `[{"type":"common","actions":["read"]}]`
	details, err := rar.DecodeForm(value)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%d details; first type = %q\n", len(details), details[0].Type())
	// Output: 1 details; first type = "common"
}

// ExampleValidateAll aggregates per-element validation across an
// [rar.AuthorizationDetails] slice. A non-nil result wraps each
// failing element with its zero-based index; the leaf errors are
// [*rar.ValidationError] values recoverable with errors.As.
func ExampleValidateAll() {
	bad1 := &rar.CommonType{} // empty TypeName fails "type-required"
	bad2 := &rar.CommonType{TypeName: "common"}
	bad2.Locations = []string{"not-a-uri"} // fails "locations-uri"
	err := rar.ValidateAll(rar.AuthorizationDetails{bad1, bad2})
	fmt.Println(err != nil)
	// Output: true
}
