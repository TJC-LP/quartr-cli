package cli

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
