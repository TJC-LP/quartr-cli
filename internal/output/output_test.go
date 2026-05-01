package output

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestChooseFieldsPrefersKnownFields(t *testing.T) {
	fields := chooseFields([]map[string]any{{
		"zzz":  "last",
		"name": "Apple Inc.",
		"id":   json.Number("4742"),
	}}, nil)

	want := []string{"id", "name"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("expected %#v, got %#v", want, fields)
	}
}

func TestWriteCSVUsesExplicitFields(t *testing.T) {
	obj := map[string]any{
		"data": []any{
			map[string]any{
				"id":   json.Number("4742"),
				"name": "Apple Inc.",
				"skip": "ignored",
			},
		},
	}

	var b bytes.Buffer
	err := Write(&b, obj, Options{Format: "csv", Fields: []string{"id", "name"}})
	if err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(b.String())
	want := "id,name\n4742,Apple Inc."
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
