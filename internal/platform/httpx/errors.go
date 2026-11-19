package httpx

import (
	"net/http"
)

// Error codes returned to clients. They are stable identifiers the frontend
// can branch on, unlike messages, which are for people.
const (
	CodeBadRequest       = "bad_request"
	CodeValidationFailed = "validation_failed"
	CodeNotFound         = "not_found"
	CodeConflict         = "conflict"
	CodeRequestTooLarge  = "request_too_large"
	CodeMethodNotAllowed = "method_not_allowed"
	CodeInternal         = "internal_error"

	CodeWorkspaceHeaderMissing = "workspace_header_missing"
	CodeWorkspaceHeaderInvalid = "workspace_header_invalid"
	CodeWorkspaceNotFound      = "workspace_not_found"
)

// BadRequest reports a malformed request: unparsable JSON, a bad query
// parameter, or anything the client can fix by changing the request itself.
func BadRequest(w http.ResponseWriter, message string) {
	Error(w, http.StatusBadRequest, CodeBadRequest, message, nil)
}

// ValidationFailed reports field-level validation errors. Details carries the
// failing fields so a form can highlight them.
func ValidationFailed(w http.ResponseWriter, details any) {
	Error(w, http.StatusUnprocessableEntity, CodeValidationFailed,
		"The request contains invalid fields.", details)
}

// NotFound reports a missing resource. It is also the correct answer for a
// resource that belongs to another workspace: saying "forbidden" would
// confirm that the resource exists.
func NotFound(w http.ResponseWriter, message string) {
	if message == "" {
		message = "The requested resource was not found."
	}
	Error(w, http.StatusNotFound, CodeNotFound, message, nil)
}

// Conflict reports a request that cannot proceed in the resource's current
// state, such as accepting a request that is no longer new.
func Conflict(w http.ResponseWriter, message string) {
	Error(w, http.StatusConflict, CodeConflict, message, nil)
}

// Internal reports a server-side failure. The message is deliberately generic:
// the cause belongs in the logs, never in the response.
func Internal(w http.ResponseWriter) {
	Error(w, http.StatusInternalServerError, CodeInternal,
		"Something went wrong. Please try again.", nil)
}
