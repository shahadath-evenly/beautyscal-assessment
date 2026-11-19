package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/real-musafir/b-scale/backend/internal/platform/httpx"
)

func TestEscapeLike(t *testing.T) {
	cases := map[string]string{
		"rose":    "rose",
		"%":       `\%`,
		"_":       `\_`,
		"50% off": `50\% off`,
		"a_b":     `a\_b`,
		`back\sl`: `back\\sl`,
		"%_mix%":  `\%\_mix\%`,
	}
	for in, want := range cases {
		if got := httpx.EscapeLike(in); got != want {
			t.Errorf("EscapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParsePageParamsDefaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)

	params, err := httpx.ParsePageParams(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Page != httpx.DefaultPage || params.Limit != httpx.DefaultLimit {
		t.Errorf("defaults = page %d limit %d, want %d and %d",
			params.Page, params.Limit, httpx.DefaultPage, httpx.DefaultLimit)
	}
	if params.SearchOrNil() != nil {
		t.Error("an absent search must bind as nil, meaning no filter")
	}
}

func TestParsePageParamsOffset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/products?page=4&limit=25", nil)

	params, err := httpx.ParsePageParams(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params.Offset() != 75 {
		t.Errorf("offset = %d, want 75", params.Offset())
	}
	if params.LimitValue() != 25 {
		t.Errorf("limit = %d, want 25", params.LimitValue())
	}
}

func TestNewMeta(t *testing.T) {
	meta := httpx.NewMeta(httpx.PageParams{Page: 2, Limit: 10}, 25)
	if meta.Page != 2 || meta.Limit != 10 || meta.Total != 25 || meta.TotalPages != 3 {
		t.Errorf("meta = %+v, want page 2, limit 10, total 25, totalPages 3", meta)
	}

	empty := httpx.NewMeta(httpx.PageParams{Page: 1, Limit: 10}, 0)
	if empty.TotalPages != 1 {
		t.Errorf("empty totalPages = %d, want 1", empty.TotalPages)
	}
}
