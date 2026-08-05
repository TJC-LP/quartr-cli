package cli

import (
	"errors"
	"fmt"
	"net/http"

	"quartr-cli/internal/quartr"
)

// usageError marks a failure caused by the command line the user typed
// rather than by the API or the network. Run maps it to exit code 2 so
// scripts can tell "you asked for something impossible" apart from "the
// request failed".
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, args ...any) error {
	return &usageError{msg: fmt.Sprintf(format, args...)}
}

// errorHint returns an extra line to print under an API error, for the two
// statuses users reliably misread.
//
// Quartr answers a tier-gated endpoint with a bare {"message":"Forbidden"},
// which is indistinguishable from a credentials problem — and since the rest
// of the CLI keeps working on the same key, the natural conclusion is that
// auth is broken. It is not: 403 is entitlement, 401 is authentication.
func errorHint(err error) string {
	var apiErr *quartr.APIError
	if !errors.As(err, &apiErr) {
		return ""
	}
	switch apiErr.StatusCode {
	case http.StatusForbidden:
		return "hint: 403 means this endpoint is not included in your API tier, not that your key is wrong " +
			"(a rejected key returns 401). Every other endpoint keeps working with the same key. " +
			"Endpoints seen gated this way: `events summary`, `audio list`, `live transcripts list`."
	case http.StatusUnauthorized:
		return "hint: 401 means the API key was rejected. Check `quartr auth show`, QUARTR_API_KEY, " +
			"and any --api-key flag."
	default:
		return ""
	}
}
