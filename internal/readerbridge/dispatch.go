// Package readerbridge validates calls from authenticated detached reader windows.
package readerbridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// Dispatch invokes only an explicitly supplied operation, with its exact typed
// arguments. No arbitrary App method name or reflection on App is exposed.
func Dispatch(methods map[string]any, operation string, args json.RawMessage) (any, error) {
	method, ok := methods[operation]
	if !ok {
		return nil, errors.New("operation unavailable in reader window")
	}
	if len(args) > 1<<20 {
		return nil, errors.New("reader request too large")
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return nil, errors.New("invalid reader arguments")
	}
	fn := reflect.ValueOf(method)
	typ := fn.Type()
	if typ.NumIn() != len(raw) {
		return nil, errors.New("incorrect reader argument count")
	}
	values := make([]reflect.Value, len(raw))
	for i, arg := range raw {
		if bytes.Equal(bytes.TrimSpace(arg), []byte("null")) {
			return nil, errors.New("reader arguments cannot be null")
		}
		value := reflect.New(typ.In(i))
		if err := json.Unmarshal(arg, value.Interface()); err != nil {
			return nil, fmt.Errorf("invalid reader argument %d", i)
		}
		values[i] = value.Elem()
	}
	results := fn.Call(values)
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if len(results) > 0 && results[len(results)-1].Type().Implements(errorType) {
		last := results[len(results)-1]
		if !last.IsNil() {
			return nil, last.Interface().(error)
		}
		results = results[:len(results)-1]
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0].Interface(), nil
}
