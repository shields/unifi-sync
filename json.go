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
