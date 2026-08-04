package cli

import (
	"fmt"
	"strings"
)

type paramSet []string

func params(xs ...string) paramSet {
	return paramSet(xs)
}

func mergeParams(sets ...paramSet) paramSet {
	seen := make(map[string]bool)
	out := paramSet{}
	for _, set := range sets {
		for _, name := range set {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func (s paramSet) allows(name string) bool {
	for _, item := range s {
		if item == name {
			return true
		}
	}
	return false
}

type resource struct {
	name          string
	listPath      string
	getPath       string
	summaryPath   string
	pagesPath     string
	chaptersPath  string
	downloadField string
	streamField   string
	listParams    paramSet
	getParams     paramSet
	summaryParams paramSet
	// sortFields lists the values the endpoint accepts for sortBy. Empty
	// means the endpoint has no sortBy parameter at all, which the CLI
	// reports instead of dropping the flag on the floor.
	sortFields paramSet
}

var (
	baseListParams = params(
		"countries",
		"exchanges",
		"tickers",
		"isins",
		"ciks",
		"companyIds",
		"startDate",
		"endDate",
		"updatedAfter",
		"updatedBefore",
		"limit",
		"cursor",
		"direction",
	)
	docListParams       = mergeParams(baseListParams, params("typeIds", "eventIds", "documentGroupIds", "expand"))
	audioListParams     = mergeParams(baseListParams, params("eventIds", "expand"))
	liveListParams      = mergeParams(params("countries", "exchanges", "tickers", "isins", "ciks", "companyIds", "eventIds", "states", "startDate", "endDate", "updatedAfter", "updatedBefore", "limit", "cursor", "direction"), params("transcriptVersion"))
	liveAudioListParams = params("countries", "exchanges", "tickers", "isins", "ciks", "companyIds", "eventIds", "states", "startDate", "endDate", "updatedAfter", "updatedBefore", "limit", "cursor", "direction")
	simpleListParams    = params("limit", "cursor", "direction")
	companyListParams   = params("countries", "exchanges", "tickers", "isins", "ciks", "ids", "updatedAfter", "updatedBefore", "limit", "cursor", "direction")
	summaryParams       = params("length", "plain")
	getExpandParams     = params("expand")
	getLiveParams       = params("transcriptVersion")

	// eventSortFields mirrors the enum the API reports when sortBy is
	// invalid: "sortBy must be one of the following values: id, date".
	// /events is the only list endpoint that accepts the parameter.
	eventSortFields = params("id", "date")
)

var resources = map[string]resource{
	"companies": {
		name:       "companies",
		listPath:   "/companies",
		getPath:    "/companies/{id}",
		listParams: companyListParams,
	},
	"events": {
		name:          "events",
		listPath:      "/events",
		getPath:       "/events/{id}",
		summaryPath:   "/events/{id}/summary",
		listParams:    mergeParams(baseListParams, params("typeIds", "sortBy")),
		summaryParams: summaryParams,
		sortFields:    eventSortFields,
	},
	"documents": {
		name:          "documents",
		listPath:      "/documents",
		getPath:       "/documents/{id}",
		downloadField: "fileUrl",
		listParams:    docListParams,
		getParams:     getExpandParams,
	},
	"reports": {
		name:          "reports",
		listPath:      "/documents/reports",
		getPath:       "/documents/reports/{id}",
		pagesPath:     "/documents/reports/{id}/pages",
		summaryPath:   "/documents/reports/{id}/summary",
		downloadField: "fileUrl",
		listParams:    docListParams,
		getParams:     getExpandParams,
		summaryParams: summaryParams,
	},
	"slides": {
		name:          "slides",
		listPath:      "/documents/slides",
		getPath:       "/documents/slides/{id}",
		pagesPath:     "/documents/slides/{id}/pages",
		summaryPath:   "/documents/slides/{id}/summary",
		downloadField: "fileUrl",
		listParams:    docListParams,
		getParams:     getExpandParams,
		summaryParams: summaryParams,
	},
	"transcripts": {
		name:          "transcripts",
		listPath:      "/documents/transcripts",
		getPath:       "/documents/transcripts/{id}",
		chaptersPath:  "/documents/transcripts/{id}/chapters",
		summaryPath:   "/documents/transcripts/{id}/summary",
		downloadField: "fileUrl",
		listParams:    docListParams,
		getParams:     getExpandParams,
		summaryParams: summaryParams,
	},
	"audio": {
		name:          "audio",
		listPath:      "/audio",
		getPath:       "/audio/{id}",
		chaptersPath:  "/audio/{id}/chapters",
		downloadField: "fileUrl",
		listParams:    audioListParams,
		getParams:     getExpandParams,
	},
	"live": {
		name:       "live",
		listPath:   "/live",
		getPath:    "/live/{id}",
		listParams: liveListParams,
		getParams:  getLiveParams,
	},
	"live-audio": {
		name:          "live-audio",
		listPath:      "/live/audio",
		getPath:       "/live/audio/{id}",
		downloadField: "audio",
		listParams:    liveAudioListParams,
	},
	"live-transcripts": {
		name:          "live-transcripts",
		listPath:      "/live/transcripts",
		getPath:       "/live/transcripts/{id}",
		downloadField: "transcript",
		streamField:   "transcript",
		listParams:    liveListParams,
		getParams:     getLiveParams,
	},
	"event-types": {
		name:       "event-types",
		listPath:   "/event-types",
		getPath:    "/event-types/{id}",
		listParams: simpleListParams,
	},
	"document-types": {
		name:       "document-types",
		listPath:   "/document-types",
		getPath:    "/document-types/{id}",
		listParams: simpleListParams,
	},
}

func resourceByName(name string) (resource, bool) {
	r, ok := resources[name]
	return r, ok
}

// validateSortBy rejects --sort-by on list endpoints that have no sortBy
// parameter. Forwarding it is a 400 and dropping it is worse: rows come back
// in insertion order, so a caller who trusts the flag silently reads stale
// documents off the first page.
func validateSortBy(r resource, sortBy string) error {
	sortBy = strings.TrimSpace(sortBy)
	if sortBy == "" {
		return nil
	}
	if len(r.sortFields) > 0 {
		if r.sortFields.allows(sortBy) {
			return nil
		}
		return usagef("--sort-by %s is not supported by `quartr %s list`; supported sort fields: %s",
			sortBy, r.name, strings.Join(r.sortFields, ", "))
	}
	return usagef("--sort-by is not supported by `quartr %s list`: the Quartr endpoint has no sortBy "+
		"parameter, so rows come back in insertion order and the newest items may be missing from the "+
		"first page.\n\n%s", r.name, sortRecipe(r))
}

// sortRecipe is the "do this instead" paragraph shown both by the --sort-by
// error and by `quartr <resource> --help`.
func sortRecipe(r resource) string {
	const directionNote = "`--direction asc|desc` is accepted here, but it reverses insertion order, not date order."
	if !r.listParams.allows("eventIds") {
		return "Only `quartr events list` supports --sort-by (fields: " +
			strings.Join(eventSortFields, ", ") + ").\n" + directionNote
	}
	return fmt.Sprintf(`Sort events first, then fetch by event id:
  quartr events list --tickers AAPL --sort-by date --direction desc --limit 5
  quartr %s list --event-ids <id>

%s`, r.name, directionNote)
}
