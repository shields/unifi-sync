package main

import (
	"math"
	"strings"
	"testing"
)

func TestDecodeJSON(t *testing.T) {
	input := `{"name":"HomeNet","vlan":100,"enabled":true}`
	obj, err := decodeJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("decodeJSON() error = %v", err)
	}
	if obj["name"] != "HomeNet" {
		t.Errorf("name = %v, want HomeNet", obj["name"])
	}
	// UseNumber preserves integers as json.Number
	vlan, ok := obj["vlan"].(jsonNumber)
	if !ok {
		t.Fatalf("vlan type = %T, want json.Number", obj["vlan"])
	}
	if vlan.String() != "100" {
		t.Errorf("vlan = %s, want 100", vlan)
	}
}

func TestDecodeJSONTrailingData(t *testing.T) {
	_, err := decodeJSON(strings.NewReader(`{"a":1}{"b":2}`))
	if err == nil {
		t.Error("decodeJSON(trailing data) should return error")
	}
}

func TestDecodeJSONInvalid(t *testing.T) {
	_, err := decodeJSON(strings.NewReader(`{invalid`))
	if err == nil {
		t.Error("decodeJSON(invalid) should return error")
	}
}

func TestDecodeJSONArray(t *testing.T) {
	input := `{"meta":{"rc":"ok"},"data":[{"name":"A"},{"name":"B"}]}`
	arr, err := decodeDataEnvelope(strings.NewReader(input))
	if err != nil {
		t.Fatalf("decodeDataEnvelope() error = %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("len = %d, want 2", len(arr))
	}
	if arr[0]["name"] != "A" || arr[1]["name"] != "B" {
		t.Errorf("got %v, want A and B", arr)
	}
}

func TestDecodeDataEnvelopeTrailingData(t *testing.T) {
	_, err := decodeDataEnvelope(strings.NewReader(`{"data":[]}{"extra":1}`))
	if err == nil {
		t.Error("decodeDataEnvelope(trailing data) should return error")
	}
}

func TestDecodeJSONArrayInvalid(t *testing.T) {
	_, err := decodeDataEnvelope(strings.NewReader(`{invalid`))
	if err == nil {
		t.Error("decodeDataEnvelope(invalid) should return error")
	}
}

func TestDecodeJSONArrayMissingData(t *testing.T) {
	_, err := decodeDataEnvelope(strings.NewReader(`{"meta":{"rc":"ok"}}`))
	if err == nil {
		t.Error("decodeDataEnvelope(no data) should return error")
	}
}

func TestDecodeJSONArrayBadDataType(t *testing.T) {
	_, err := decodeDataEnvelope(strings.NewReader(`{"data":"not-an-array"}`))
	if err == nil {
		t.Error("decodeDataEnvelope(data not array) should return error")
	}
}

func TestDecodeJSONArrayBadElement(t *testing.T) {
	_, err := decodeDataEnvelope(strings.NewReader(`{"data":["not-an-object"]}`))
	if err == nil {
		t.Error("decodeDataEnvelope(bad element) should return error")
	}
}

func TestMarshalJSON(t *testing.T) {
	obj := map[string]any{
		"b": "two",
		"a": "one",
	}
	data, err := marshalJSON(obj)
	if err != nil {
		t.Fatalf("marshalJSON() error = %v", err)
	}
	got := string(data)
	// Keys sorted alphabetically, 2-space indent, trailing newline
	want := "{\n  \"a\": \"one\",\n  \"b\": \"two\"\n}\n"
	if got != want {
		t.Errorf("marshalJSON() =\n%s\nwant:\n%s", got, want)
	}
}

func TestMarshalJSONError(t *testing.T) {
	obj := map[string]any{"bad": math.Inf(1)}
	_, err := marshalJSON(obj)
	if err == nil {
		t.Error("marshalJSON(Inf) should return error")
	}
}

func TestMarshalJSONPreservesNumbers(t *testing.T) {
	input := `{"port":8443,"ratio":1.5}`
	obj, err := decodeJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("decodeJSON() error = %v", err)
	}
	data, err := marshalJSON(obj)
	if err != nil {
		t.Fatalf("marshalJSON() error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "8443") {
		t.Errorf("integer not preserved: %s", got)
	}
	if !strings.Contains(got, "1.5") {
		t.Errorf("float not preserved: %s", got)
	}
}
