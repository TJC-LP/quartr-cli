package cli

import "testing"

func TestNextCursor(t *testing.T) {
	obj := map[string]any{
		"pagination": map[string]any{
			"nextCursor": "next-page",
		},
	}
	if got := nextCursor(obj); got != "next-page" {
		t.Fatalf("expected next-page, got %q", got)
	}

	obj["pagination"] = map[string]any{"nextCursor": nil}
	if got := nextCursor(obj); got != "" {
		t.Fatalf("expected empty cursor, got %q", got)
	}
}

func TestExtractStringFieldFallsBackToData(t *testing.T) {
	obj := map[string]any{
		"data": map[string]any{
			"fileUrl": "https://example.com/file.pdf",
		},
	}

	got, err := extractStringField(obj, "fileUrl")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/file.pdf" {
		t.Fatalf("expected data fileUrl, got %q", got)
	}
}

func TestDefaultFileNameSanitizesAndKeepsExtension(t *testing.T) {
	got := defaultFileName("live transcripts", "abc/123", "https://example.com/path/file.json?x=1")
	want := "live-transcripts-abc-123.json"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
