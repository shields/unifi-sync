// Copyright © 2026 Michael Shields
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type jsonNumber = json.Number

func decodeJSON(r io.Reader) (map[string]any, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("unexpected trailing data after JSON object")
	}
	return obj, nil
}

// decodeDataEnvelope decodes a UniFi API response envelope {"meta":...,"data":[...]}
// and returns the data array as a slice of objects.
func decodeDataEnvelope(r io.Reader) ([]map[string]any, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var envelope map[string]any
	if err := dec.Decode(&envelope); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("unexpected trailing data after JSON object")
	}
	// The controller signals API-level failures via meta.rc, often with an
	// HTTP 200 status, so surface them rather than treating the response as
	// success.
	if meta, ok := envelope["meta"].(map[string]any); ok {
		rc, _ := meta["rc"].(string) //nolint:errcheck // missing/non-string rc means no error signal
		if rc != "" && rc != "ok" {
			msg, _ := meta["msg"].(string) //nolint:errcheck // missing/non-string msg is omitted from the error
			if msg != "" {
				return nil, fmt.Errorf("controller API error: %s", msg)
			}
			return nil, fmt.Errorf("controller API error (rc=%q)", rc)
		}
	}
	rawData, ok := envelope["data"]
	if !ok {
		return nil, errors.New("response missing \"data\" field")
	}
	arr, ok := rawData.([]any)
	if !ok {
		return nil, errors.New("\"data\" field is not an array")
	}
	result := make([]map[string]any, len(arr))
	for i, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("data[%d] is not an object", i)
		}
		result[i] = obj
	}
	return result, nil
}

// deepCopyJSONObject returns a deep copy of a JSON-decoded object so that
// mutating the copy — for example, injecting secrets into nested arrays — never
// affects the original.
func deepCopyJSONObject(obj map[string]any) map[string]any {
	cp := make(map[string]any, len(obj))
	for k, v := range obj {
		cp[k] = deepCopyJSONValue(v)
	}
	return cp
}

// deepCopyJSONValue deep-copies a value decoded from JSON. Maps and slices are
// copied recursively; scalars (string, bool, json.Number, nil) are immutable and
// returned as-is.
func deepCopyJSONValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCopyJSONObject(val)
	case []any:
		cp := make([]any, len(val))
		for i, e := range val {
			cp[i] = deepCopyJSONValue(e)
		}
		return cp
	default:
		return val
	}
}

// marshalJSON produces 2-space indented JSON with a trailing newline,
// matching the on-disk config file format per spec.
func marshalJSON(obj map[string]any) ([]byte, error) {
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	return data, nil
}
