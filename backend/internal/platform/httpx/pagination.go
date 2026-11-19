package httpx

import (
	"net/http"
	"strconv"
	"strings"
)

// Pagination defaults and bounds, shared by every listing endpoint.
const (
	DefaultPage  = 1
	DefaultLimit = 10
	MaxLimit     = 100
	// MaxSearchLength bounds the search term, so a huge string cannot be
	// pushed into a LIKE pattern.
	MaxSearchLength = 200
)

// PageParams are the pagination and search inputs common to all listings.
type PageParams struct {
	Page   int    `validate:"min=1"`
	Limit  int    `validate:"min=1,max=100"`
	Search string `validate:"max=200"`
}

// Offset is the SQL OFFSET for these parameters.
func (p PageParams) Offset() int32 { return int32((p.Page - 1) * p.Limit) }

// LimitValue is the SQL LIMIT for these parameters.
func (p PageParams) LimitValue() int32 { return int32(p.Limit) }

// SearchOrNil returns the search term ready to bind, or nil when empty,
// matching the generated queries where a nil filter means "no filter".
//
// The term is bound as a parameter and the LIKE pattern is assembled by
// PostgreSQL, so nothing is concatenated into a query string. LIKE
// metacharacters are escaped here so they are matched literally: without it,
// a search for "%" would produce the pattern "%%%" and return every row,
// and "_" would match any single character.
func (p PageParams) SearchOrNil() *string {
	if p.Search == "" {
		return nil
	}
	search := EscapeLike(p.Search)
	return &search
}

// EscapeLike escapes the characters that are special inside a LIKE or ILIKE
// pattern, using the backslash escape character PostgreSQL applies by
// default. The backslash itself is escaped first, or escaping the others
// would double-escape it.
func EscapeLike(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		"%", `\%`,
		"_", `\_`,
	)
	return replacer.Replace(s)
}

// ParsePageParams reads page, limit and search from the query string.
//
// Absent parameters take their defaults. A present but unusable value is an
// error rather than a silent fallback: ?limit=abc or ?page=-1 is a client
// mistake, and quietly returning page 1 would hide it.
func ParsePageParams(r *http.Request) (PageParams, *FieldError) {
	query := r.URL.Query()

	page, fieldErr := intParam(query.Get("page"), "page", DefaultPage)
	if fieldErr != nil {
		return PageParams{}, fieldErr
	}
	if page < 1 {
		return PageParams{}, &FieldError{Field: "page", Message: "must be 1 or greater"}
	}

	limit, fieldErr := intParam(query.Get("limit"), "limit", DefaultLimit)
	if fieldErr != nil {
		return PageParams{}, fieldErr
	}
	switch {
	case limit < 1:
		return PageParams{}, &FieldError{Field: "limit", Message: "must be 1 or greater"}
	case limit > MaxLimit:
		return PageParams{}, &FieldError{
			Field:   "limit",
			Message: "must be " + strconv.Itoa(MaxLimit) + " or fewer",
		}
	}

	search := strings.TrimSpace(query.Get("search"))
	if len(search) > MaxSearchLength {
		return PageParams{}, &FieldError{
			Field:   "search",
			Message: "must be " + strconv.Itoa(MaxSearchLength) + " characters or fewer",
		}
	}

	return PageParams{Page: page, Limit: limit, Search: search}, nil
}

// FieldError names one invalid input and why, for the error envelope's
// details.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// WriteTo sends the field error as a 400 with details.
func (e *FieldError) WriteTo(w http.ResponseWriter) {
	Error(w, http.StatusBadRequest, CodeBadRequest,
		"One or more query parameters are invalid.", []FieldError{*e})
}

func intParam(raw, field string, fallback int) (int, *FieldError) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, &FieldError{Field: field, Message: "must be a whole number"}
	}
	return value, nil
}

// NewMeta builds the pagination metadata for a response. TotalPages is at
// least 1, so an empty list still describes a valid page.
func NewMeta(params PageParams, total int64) Meta {
	totalPages := 1
	if total > 0 {
		totalPages = int((total + int64(params.Limit) - 1) / int64(params.Limit))
	}
	return Meta{
		Page:       params.Page,
		Limit:      params.Limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
