package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// companyJoinBatch caps how many ids go into one /companies?ids=... request.
const companyJoinBatch = 100

// splitExpand separates the client-side "company" expansion from the values
// the API understands. Quartr has no company expansion: /events rejects the
// parameter entirely ("property expand should not exist") and the document
// endpoints accept only "event". So the CLI performs that join itself rather
// than forwarding a value that is guaranteed to 400.
// It returns the expand value to forward and whether the caller asked for the
// client-side company join.
func splitExpand(expand string) (string, bool) {
	kept := make([]string, 0, 2)
	joinCompany := false
	for _, v := range parseCSV(expand) {
		if strings.EqualFold(v, "company") {
			joinCompany = true
			continue
		}
		kept = append(kept, v)
	}
	return strings.Join(kept, ","), joinCompany
}

// checkCompanyExpand reports whether --expand company makes sense for r.
func checkCompanyExpand(r resource) error {
	if r.name == "companies" {
		return usagef("--expand company is redundant for `quartr companies`; the rows already are companies")
	}
	if !r.listParams.allows("companyIds") {
		return usagef("--expand company is not available for `quartr %s`; its rows carry no companyId", r.name)
	}
	return nil
}

// joinCompanies fills in row["company"] for every row that carries a
// companyId, by batch-fetching /companies. Rows that already embed a company
// object are left alone, so this becomes a no-op if Quartr ever starts
// expanding server-side.
//
// A lookup failure is reported on stderr and leaves the rows unexpanded
// rather than failing the whole command: the caller still gets their data.
func (a *app) joinCompanies(ctx context.Context, rows []map[string]any) {
	ids := distinctCompanyIDs(rows)
	if len(ids) == 0 {
		return
	}

	byID := make(map[string]map[string]any, len(ids))
	for chunk := range slices.Chunk(ids, companyJoinBatch) {
		params := url.Values{}
		params.Set("ids", strings.Join(chunk, ","))
		params.Set("limit", strconv.Itoa(len(chunk)))
		obj, _, err := a.client.GetJSON(ctx, "/companies", params)
		if err != nil {
			fmt.Fprintf(a.errOut, "warning: --expand company: %v\n", err)
			return
		}
		for _, company := range dataRows(obj) {
			if id := idKey(company["id"]); id != "" {
				byID[id] = company
			}
		}
	}

	missing := 0
	for _, row := range rows {
		if _, ok := row["company"].(map[string]any); ok {
			continue
		}
		id := idKey(row["companyId"])
		if id == "" {
			continue
		}
		if company, ok := byID[id]; ok {
			row["company"] = company
			continue
		}
		missing++
	}
	if missing > 0 {
		fmt.Fprintf(a.errOut, "warning: --expand company: no company record for %d of %d rows\n", missing, len(rows))
	}
}

func distinctCompanyIDs(rows []map[string]any) []string {
	seen := make(map[string]bool, len(rows))
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if _, ok := row["company"].(map[string]any); ok {
			continue
		}
		id := idKey(row["companyId"])
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// idKey normalizes an id from a decoded JSON body into a comparable string.
// Bodies are decoded with UseNumber, so ids arrive as json.Number.
func idKey(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}

// joinableRows returns the mutable row maps inside a decoded response,
// handling both the list shape ({"data": [ ... ]}) and the single-object
// shape ({"data": { ... }}) returned by get endpoints.
func joinableRows(obj map[string]any) []map[string]any {
	if rows := dataRows(obj); len(rows) > 0 {
		return rows
	}
	if single, ok := obj["data"].(map[string]any); ok {
		return []map[string]any{single}
	}
	return nil
}
