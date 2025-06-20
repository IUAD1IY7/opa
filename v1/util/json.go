// Copyright 2016 The OPA Authors.  All rights reserved.
// Use of this source code is governed by an Apache2
// license that can be found in the LICENSE file.

package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"sigs.k8s.io/yaml"

	"github.com/IUAD1IY7/opa/v1/loader/extension"
)

// UnmarshalJSON parses the JSON encoded data and stores the result in the value
// pointed to by x.
//
// This function is intended to be used in place of the standard json.Marshal
// function when json.Number is required.
func UnmarshalJSON(bs []byte, x any) error {
	return unmarshalJSON(bs, x, true)
}

func unmarshalJSON(bs []byte, x any, ext bool) error {
	buf := bytes.NewBuffer(bs)
	decoder := NewJSONDecoder(buf)
	if err := decoder.Decode(x); err != nil {
		if handler := extension.FindExtension(".json"); handler != nil && ext {
			return handler(bs, x)
		}
		return err
	}

	// Since decoder.Decode validates only the first json structure in bytes,
	// check if decoder has more bytes to consume to validate whole input bytes.
	tok, err := decoder.Token()
	if tok != nil {
		return fmt.Errorf("error: invalid character '%s' after top-level value", tok)
	}
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}

// NewJSONDecoder returns a new decoder that reads from r.
//
// This function is intended to be used in place of the standard json.NewDecoder
// when json.Number is required.
func NewJSONDecoder(r io.Reader) *json.Decoder {
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	return decoder
}

// MustUnmarshalJSON parse the JSON encoded data and returns the result.
//
// If the data cannot be decoded, this function will panic. This function is for
// test purposes.
func MustUnmarshalJSON(bs []byte) any {
	var x any
	if err := UnmarshalJSON(bs, &x); err != nil {
		panic(err)
	}
	return x
}

// MustMarshalJSON returns the JSON encoding of x
//
// If the data cannot be encoded, this function will panic. This function is for
// test purposes.
func MustMarshalJSON(x any) []byte {
	bs, err := json.Marshal(x)
	if err != nil {
		panic(err)
	}
	return bs
}

// RoundTrip encodes to JSON, and decodes the result again.
//
// Thereby, it is converting its argument to the representation expected by
// rego.Input and inmem's Write operations. Works with both references and
// values.
func RoundTrip(x *any) error {
	bs, err := json.Marshal(x)
	if err != nil {
		return err
	}
	return UnmarshalJSON(bs, x)
}

// Reference returns a pointer to its argument unless the argument already is
// a pointer. If the argument is **t, or ***t, etc, it will return *t.
//
// Used for preparing Go types (including pointers to structs) into values to be
// put through util.RoundTrip().
func Reference(x any) *any {
	var y any
	rv := reflect.ValueOf(x)
	if rv.Kind() == reflect.Ptr {
		return Reference(rv.Elem().Interface())
	}
	if rv.Kind() != reflect.Invalid {
		y = rv.Interface()
		return &y
	}
	return &x
}

// Unmarshal decodes a YAML, JSON or JSON extension value into the specified type.
func Unmarshal(bs []byte, v any) error {
	if len(bs) > 2 && bs[0] == 0xef && bs[1] == 0xbb && bs[2] == 0xbf {
		bs = bs[3:] // Strip UTF-8 BOM, see https://www.rfc-editor.org/rfc/rfc8259#section-8.1
	}

	if json.Valid(bs) {
		return unmarshalJSON(bs, v, false)
	}
	nbs, err := yaml.YAMLToJSON(bs)
	if err == nil {
		return unmarshalJSON(nbs, v, false)
	}
	// not json or yaml: try extensions
	if handler := extension.FindExtension(".json"); handler != nil {
		return handler(bs, v)
	}
	return err
}
