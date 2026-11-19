package app

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestWailsBoundStructsHaveModels guards the contract between bound Go
// methods and the checked-in TypeScript bindings. Wails regenerates
// frontend/wailsjs during `wails build`, naming each model after the package
// that DEFINES the struct. A type alias (type X = other.X) silently moves the
// generated class to the other package's namespace: the committed bindings
// keep showing the old namespace, frontend type checks stay green, and only
// the built DMG breaks at runtime ("undefined is not a constructor").
//
// This test fails when a struct used in a bound method signature has no
// matching class in the committed models.ts under the namespace Wails will
// actually generate. Fix by regenerating bindings (make generate) and pointing
// the frontend at the generated namespace, or by making the type concrete in
// the bound package.
func TestWailsBoundStructsHaveModels(t *testing.T) {
	modelsPath := filepath.Join("..", "frontend", "wailsjs", "go", "models.ts")
	data, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatalf("read committed bindings: %v", err)
	}
	classes := parseWailsModelClasses(string(data))

	required := map[string]map[string]bool{}
	for _, bound := range []reflect.Type{reflect.TypeOf((*App)(nil)), reflect.TypeOf((*ReaderApp)(nil))} {
		collectBoundStructTypes(bound, required)
	}

	var missing []string
	for pkg, names := range required {
		for name := range names {
			if !classes[pkg][name] {
				missing = append(missing, pkg+"."+name)
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("bound method signatures reference structs that Wails generates outside the committed models.ts namespaces: %s", strings.Join(missing, ", "))
	}
}

var (
	wailsNamespaceRe = regexp.MustCompile(`^export namespace (\w+) \{$`)
	wailsClassRe     = regexp.MustCompile(`^\s+export class (\w+)`)
)

func parseWailsModelClasses(content string) map[string]map[string]bool {
	classes := map[string]map[string]bool{}
	namespace := ""
	for _, line := range strings.Split(content, "\n") {
		if m := wailsNamespaceRe.FindStringSubmatch(line); m != nil {
			namespace = m[1]
			if classes[namespace] == nil {
				classes[namespace] = map[string]bool{}
			}
			continue
		}
		if namespace != "" && line == "}" {
			namespace = ""
			continue
		}
		if namespace == "" {
			continue
		}
		if m := wailsClassRe.FindStringSubmatch(line); m != nil {
			classes[namespace][m[1]] = true
		}
	}
	return classes
}

// collectBoundStructTypes mirrors the parameter/result unwrapping in Wails'
// binding generator (internal/binding/reflect.go): pointers and slices are
// unwrapped, struct types are recorded under the package Wails derives from
// the reflected type name.
func collectBoundStructTypes(bound reflect.Type, out map[string]map[string]bool) {
	var record func(t reflect.Type)
	record = func(t reflect.Type) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || t.Name() == "" {
			return
		}
		if !strings.HasPrefix(t.PkgPath(), "github.com/hkdb/aerion") {
			return
		}
		// Wails names the namespace from the reflected type string, so an
		// alias resolves to the defining package, not the bound one.
		pkg := strings.Split(t.String(), ".")[0]
		if out[pkg] == nil {
			out[pkg] = map[string]bool{}
		}
		if out[pkg][t.Name()] {
			return
		}
		out[pkg][t.Name()] = true
		// The generator emits every model reachable through a bound method,
		// including structs referenced only from another model's fields, so
		// the drift check must follow exported fields too — an aliased
		// struct nested inside a bound model moves namespaces just the same.
		// Named-map/slice element types unwrap at the next record call.
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			ft := f.Type
			for ft.Kind() == reflect.Pointer || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Map {
				if ft.Kind() == reflect.Map {
					record(ft.Key())
					ft = ft.Elem()
					continue
				}
				ft = ft.Elem()
			}
			record(ft)
		}
	}
	for i := 0; i < bound.NumMethod(); i++ {
		m := bound.Method(i)
		if m.PkgPath != "" { // unexported
			continue
		}
		mt := m.Type
		for j := 1; j < mt.NumIn(); j++ { // In(0) is the receiver
			record(mt.In(j))
		}
		for j := 0; j < mt.NumOut(); j++ {
			record(mt.Out(j))
		}
	}
}
