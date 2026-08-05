package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/TJC-LP/quartr-cli/internal/output"
)

// listRequest describes one list-or-paginate call. It exists so callers can
// opt into post-fetch shaping (currently the company join) without growing
// the fetchList signature every time.
type listRequest struct {
	path        string
	params      url.Values
	all         bool
	fields      []string
	joinCompany bool
}

func (a *app) fetchList(req listRequest) error {
	ctx := context.Background()
	path, params := req.path, req.params
	if !req.all {
		obj, _, err := a.client.GetJSON(ctx, path, params)
		if err != nil {
			return err
		}
		if req.joinCompany {
			// dataRows hands back the same maps the response holds, so
			// filling them in updates obj.
			a.joinCompanies(ctx, dataRows(obj))
		}
		return output.Write(a.out, obj, output.Options{Format: a.cfg.Format(), Fields: req.fields})
	}

	allRows := make([]map[string]any, 0)
	var finalPagination any
	seen := map[string]bool{}
	for {
		cursor := params.Get("cursor")
		if cursor != "" {
			if seen[cursor] {
				return fmt.Errorf("pagination loop detected at cursor %q", cursor)
			}
			seen[cursor] = true
		}

		obj, _, err := a.client.GetJSON(ctx, path, params)
		if err != nil {
			return err
		}
		if p, ok := obj["pagination"]; ok {
			finalPagination = p
		}
		allRows = append(allRows, dataRows(obj)...)

		next := nextCursor(obj)
		if next == "" {
			break
		}
		params.Set("cursor", next)
	}

	if req.joinCompany {
		// One join across every page, so a 5-page pull is still one
		// /companies round-trip per 100 distinct ids.
		a.joinCompanies(ctx, allRows)
	}
	wrapped := map[string]any{"data": allRows, "pagination": finalPagination, "count": len(allRows)}
	return output.Write(a.out, wrapped, output.Options{Format: a.cfg.Format(), Fields: req.fields})
}

func dataRows(obj map[string]any) []map[string]any {
	data, ok := obj["data"]
	if !ok {
		return nil
	}
	arr, ok := data.([]any)
	if !ok {
		return nil
	}

	rows := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if m, ok := v.(map[string]any); ok {
			rows = append(rows, m)
		}
	}
	return rows
}

func nextCursor(obj map[string]any) string {
	p, ok := obj["pagination"].(map[string]any)
	if !ok {
		return ""
	}
	v := p["nextCursor"]
	if v == nil {
		return ""
	}
	s := fmt.Sprintf("%v", v)
	if s == "<nil>" || s == "" {
		return ""
	}
	return s
}

func extractStringField(obj map[string]any, field string) (string, error) {
	if strings.TrimSpace(field) == "" {
		return "", errors.New("empty url field")
	}
	s, err := extractStringFieldOnce(obj, field)
	if err == nil {
		return s, nil
	}
	// Common Quartr shape is {"data": {"fileUrl": "..."}}. Retry under data
	// when caller passed a top-level field.
	if !strings.HasPrefix(field, "data.") {
		if s, retryErr := extractStringFieldOnce(obj, "data."+field); retryErr == nil {
			return s, nil
		}
	}
	return "", err
}

func extractStringFieldOnce(obj map[string]any, field string) (string, error) {
	cur := any(obj)
	for part := range strings.SplitSeq(field, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", fmt.Errorf("field %q not found", field)
		}
		cur = m[part]
		if cur == nil {
			return "", fmt.Errorf("field %q not found", field)
		}
	}
	if s, ok := cur.(string); ok && strings.TrimSpace(s) != "" {
		return s, nil
	}
	return "", fmt.Errorf("field %q is not a non-empty string", field)
}

var unsafeFilename = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func defaultFileName(resourceName, id, rawURL string) string {
	ext := ""
	if u, err := url.Parse(rawURL); err == nil {
		ext = filepath.Ext(u.Path)
	}
	if ext == "" {
		ext = ".bin"
	}
	base := unsafeFilename.ReplaceAllString(resourceName+"-"+id, "-")
	return base + ext
}
