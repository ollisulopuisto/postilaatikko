package app

import (
	"context"
	"encoding/json"
	"github.com/hkdb/aerion/internal/database"
	"github.com/hkdb/aerion/internal/readerbridge"
	"github.com/hkdb/aerion/internal/settings"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hkdb/aerion/internal/ipc"
)

func TestDetachedEmailTemplateOperationsUseParent(t *testing.T) {
	cases := []struct{ name, args, response string }{
		{"ListEmailTemplates", `[]`, `[{"id":"one","name":"Greeting","subject":"Hello","body":"Body"}]`},
		{"SaveEmailTemplate", `[{"id":"one","name":"Greeting","subject":"Hello","body":"Body"}]`, `{"id":"one","name":"Greeting","subject":"Hello","body":"Body"}`},
		{"DeleteEmailTemplate", `["one"]`, `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &readerTestClient{response: ipc.Message{Payload: json.RawMessage(tc.response)}}
			child := NewComposerApp(ComposerConfig{}, nil)
			child.ctx = context.Background()
			child.ipcClient = client
			method := reflect.ValueOf(child).MethodByName(tc.name)
			if !method.IsValid() {
				t.Fatalf("detached composer lacks %s", tc.name)
			}
			typ := method.Type()
			var raw []json.RawMessage
			if err := json.Unmarshal([]byte(tc.args), &raw); err != nil {
				t.Fatal(err)
			}
			inputs := make([]reflect.Value, len(raw))
			for i, v := range raw {
				arg := reflect.New(typ.In(i))
				if err := json.Unmarshal(v, arg.Interface()); err != nil {
					t.Fatal(err)
				}
				inputs[i] = arg.Elem()
			}
			results := method.Call(inputs)
			if !results[len(results)-1].IsNil() {
				t.Fatal(results[len(results)-1].Interface())
			}
			var request struct {
				Operation string          `json:"operation"`
				Args      json.RawMessage `json:"args"`
			}
			if err := client.request.ParsePayload(&request); err != nil {
				t.Fatal(err)
			}
			var want, got any
			if err := json.Unmarshal([]byte(tc.args), &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(request.Args, &got); err != nil {
				t.Fatal(err)
			}
			// A no-argument parent call may encode its variadic arguments as null.
			if len(raw) > 0 && !reflect.DeepEqual(want, got) {
				t.Fatalf("arguments %s", request.Args)
			}
			if len(results) > 1 {
				encoded, err := json.Marshal(results[0].Interface())
				if err != nil {
					t.Fatal(err)
				}
				var actual, expected any
				if err = json.Unmarshal(encoded, &actual); err != nil {
					t.Fatal(err)
				}
				if err = json.Unmarshal([]byte(tc.response), &expected); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(actual, expected) {
					t.Fatalf("response not preserved: %s", encoded)
				}
			}
			if request.Operation != tc.name || client.request.Type != composerCallType {
				t.Fatalf("wrong route: %+v", request)
			}
			client.response = ipc.Message{Error: "template store unavailable"}
			results = method.Call(inputs)
			if results[len(results)-1].IsNil() {
				t.Fatal("parent failure hidden")
			}
			if !strings.Contains(results[len(results)-1].Interface().(error).Error(), "template store unavailable") {
				t.Fatal(results)
			}
		})
	}
}

func TestEmailTemplateParentOperationsAreExplicitlyAllowlisted(t *testing.T) {
	methods := (&App{}).readerMethods()
	for _, name := range []string{"ListEmailTemplates", "SaveEmailTemplate", "DeleteEmailTemplate"} {
		if _, ok := methods[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	for _, name := range []string{"SetSetting", "RemoveAccount", "QuitApp"} {
		if _, ok := methods[name]; ok {
			t.Errorf("unrelated operation exposed: %s", name)
		}
	}
}

func TestTemplateParentDispatchPersistsAndValidates(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "templates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	parent := &App{settingsStore: settings.NewStore(db)}
	dispatch := func(op string, args any) (any, error) {
		data, e := json.Marshal(args)
		if e != nil {
			t.Fatal(e)
		}
		return readerbridge.Dispatch(parent.readerMethods(), op, data)
	}
	value, err := dispatch("SaveEmailTemplate", []any{settings.EmailTemplate{Name: "Greeting", Subject: "Hello", Body: "Body"}})
	if err != nil {
		t.Fatal(err)
	}
	saved := value.(settings.EmailTemplate)
	if saved.ID == "" {
		t.Fatal("missing template ID")
	}
	for _, input := range []settings.EmailTemplate{{Name: "Invalid"}, {Name: "Too large", Body: strings.Repeat("x", 65537)}} {
		if _, err = dispatch("SaveEmailTemplate", []any{input}); err == nil {
			t.Fatal("invalid template accepted")
		}
	}
	value, err = dispatch("ListEmailTemplates", []any{})
	if err != nil {
		t.Fatal(err)
	}
	got := value.([]settings.EmailTemplate)
	if len(got) != 1 || got[0] != saved {
		t.Fatalf("persisted templates: %#v", got)
	}
	if _, err = dispatch("DeleteEmailTemplate", []any{saved.ID}); err != nil {
		t.Fatal(err)
	}
	value, err = dispatch("ListEmailTemplates", []any{})
	if err != nil || len(value.([]settings.EmailTemplate)) != 0 {
		t.Fatalf("delete: %v %v", value, err)
	}
}
