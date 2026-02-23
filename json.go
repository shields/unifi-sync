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
