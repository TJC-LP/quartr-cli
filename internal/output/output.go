// Package output formats CLI responses as table, json, csv, or raw.
// Table and CSV output respect Options.Fields, including dotted paths
// like "event.title" that traverse nested maps.
package output

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
)

// Options controls how Write renders an object.
//
// Format is one of "table" (default), "json", "csv", or "raw".
// Fields is the explicit column list for table/csv output; empty means
// auto-select preferred columns from the response.
type Options struct {
	Format string
	Fields []string
}

// Write renders obj to w according to opts.Format. Returns an error for
// unknown formats or write failures.
func Write(w io.Writer, obj any, opts Options) error {
	format := strings.ToLower(strings.TrimSpace(opts.Format))
	if format == "" {
		format = "table"
	}
	switch format {
	case "json":
		return writeJSON(w, obj)
	case "raw":
		return writeRaw(w, obj)
	case "csv":
		return writeCSV(w, obj, opts.Fields)
	case "table":
		return writeTable(w, obj, opts.Fields)
	default:
		return fmt.Errorf("unknown format %q; use table, json, csv, or raw", opts.Format)
	}
}

func writeJSON(w io.Writer, obj any) error {
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

func writeRaw(w io.Writer, obj any) error {
	switch v := obj.(type) {
	case []byte:
		_, err := w.Write(v)
		if err == nil && len(v) > 0 && v[len(v)-1] != '\n' {
			_, err = w.Write([]byte("\n"))
		}
		return err
	case string:
		_, err := fmt.Fprintln(w, v)
		return err
	default:
		b, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		_, err = w.Write(append(b, '\n'))
		return err
	}
}

func writeCSV(w io.Writer, obj any, fields []string) error {
	rows, isList := extractRows(obj)
	cw := csv.NewWriter(w)
	if isList {
		if len(rows) == 0 {
			return nil
		}
		fields = chooseFields(rows, fields)
		if err := cw.Write(fields); err != nil {
			return err
		}
		for _, row := range rows {
			rec := make([]string, len(fields))
			for i, f := range fields {
				rec[i] = renderValue(getPath(row, f), 0)
			}
			if err := cw.Write(rec); err != nil {
				return err
			}
		}
	} else {
		m := asMap(obj)
		if m == nil {
			return cw.Write([]string{renderValue(obj, 0)})
		}
		fields = chooseFields([]map[string]any{m}, fields)
		if err := cw.Write([]string{"key", "value"}); err != nil {
			return err
		}
		for _, f := range fields {
			if err := cw.Write([]string{f, renderValue(getPath(m, f), 0)}); err != nil {
				return err
			}
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeTable(w io.Writer, obj any, fields []string) error {
	rows, isList := extractRows(obj)
	if isList {
		if len(rows) == 0 {
			_, err := fmt.Fprintln(w, "No rows")
			return err
		}
		fields = chooseFields(rows, fields)
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, strings.Join(fields, "\t"))
		for _, row := range rows {
			vals := make([]string, len(fields))
			for i, f := range fields {
				vals[i] = renderValue(getPath(row, f), 120)
			}
			fmt.Fprintln(tw, strings.Join(vals, "\t"))
		}
		return tw.Flush()
	}
	m := asMap(obj)
	if m == nil {
		_, err := fmt.Fprintln(w, renderValue(obj, 0))
		return err
	}
	m = unwrapDataMap(m)
	fields = chooseFields([]map[string]any{m}, fields)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	for _, f := range fields {
		fmt.Fprintf(tw, "%s\t%s\n", f, renderValue(getPath(m, f), 160))
	}
	return tw.Flush()
}

func extractRows(obj any) ([]map[string]any, bool) {
	m := asMap(obj)
	if m != nil {
		if data, ok := m["data"]; ok {
			switch arr := data.(type) {
			case []any:
				rows := make([]map[string]any, 0, len(arr))
				for _, item := range arr {
					if row := asMap(item); row != nil {
						rows = append(rows, row)
					}
				}
				return rows, true
			case []map[string]any:
				return arr, true
			}
		}
		if rowsAny, ok := m["rows"]; ok {
			if arr, ok := rowsAny.([]map[string]any); ok {
				return arr, true
			}
		}
	}
	switch arr := obj.(type) {
	case []map[string]any:
		return arr, true
	case []any:
		rows := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			if row := asMap(item); row != nil {
				rows = append(rows, row)
			}
		}
		return rows, true
	}
	return nil, false
}

func unwrapDataMap(m map[string]any) map[string]any {
	if data, ok := m["data"]; ok {
		if dm := asMap(data); dm != nil {
			return dm
		}
	}
	return m
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func chooseFields(rows []map[string]any, explicit []string) []string {
	if len(explicit) > 0 {
		return explicit
	}
	preferred := []string{
		"id", "name", "displayName", "title", "parent", "category", "form",
		"companyId", "eventId", "typeId", "documentGroupId", "fiscalYear", "fiscalPeriod",
		"date", "state", "wentLiveAt", "qna", "startTimestamp", "endTimestamp", "level",
		"fileUrl", "streamUrl", "audio", "transcript", "pdfUrl", "imageUrl", "backlinkUrl",
		"country", "tickers", "isins", "cik",
		"event.title", "event.date", "event.typeId", "event.fiscalYear", "event.fiscalPeriod",
		"updatedAt", "createdAt",
	}
	present := map[string]bool{}
	for _, row := range rows {
		for _, f := range preferred {
			if v := getPath(row, f); v != nil {
				present[f] = true
			}
		}
	}
	var fields []string
	for _, f := range preferred {
		if present[f] {
			fields = append(fields, f)
		}
	}
	if len(fields) > 0 {
		return fields
	}
	keys := map[string]bool{}
	for _, row := range rows {
		for k := range row {
			keys[k] = true
		}
	}
	for k := range keys {
		fields = append(fields, k)
	}
	slices.Sort(fields)
	return fields
}

func getPath(m map[string]any, path string) any {
	cur := any(m)
	for p := range strings.SplitSeq(path, ".") {
		cm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = cm[p]
		if cur == nil {
			return nil
		}
	}
	return cur
}

func renderValue(v any, maxLen int) string {
	if v == nil {
		return ""
	}
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case json.Number:
		s = t.String()
	case float64:
		s = strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		s = strconv.FormatFloat(float64(t), 'f', -1, 32)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		s = fmt.Sprintf("%v", t)
	case bool:
		s = strconv.FormatBool(t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			s = fmt.Sprintf("%v", t)
		} else {
			s = string(b)
		}
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	if maxLen > 0 && len(s) > maxLen {
		return s[:maxLen-1] + "…"
	}
	return s
}

// PrettyJSONBytes reformats raw JSON with 2-space indentation. Invalid
// JSON is returned unchanged so callers can use it as a best-effort
// formatter on untrusted input.
func PrettyJSONBytes(b []byte) []byte {
	var obj any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil {
		return b
	}
	pretty, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return b
	}
	return append(pretty, '\n')
}
