package validate_test

import (
	"testing"

	"github.com/real-musafir/b-scale/backend/internal/platform/validate"
)

type item struct {
	ProductID string `json:"productId" validate:"required,uuid"`
	Quantity  int32  `json:"quantity"  validate:"required,gt=0"`
}

type payload struct {
	BuyerName  string `json:"buyerName"  validate:"required,max=200"`
	BuyerEmail string `json:"buyerEmail" validate:"required,email"`
	Items      []item `json:"items"      validate:"required,min=1,dive"`
}

func TestStructReportsJSONFieldPathsAndMessages(t *testing.T) {
	v := validate.New()

	errs, ok := v.Struct(payload{
		BuyerName:  "",
		BuyerEmail: "not-an-email",
		Items:      []item{{ProductID: "not-a-uuid", Quantity: 0}},
	})
	if !ok {
		t.Fatal("validation reported a non-field failure")
	}
	if len(errs) == 0 {
		t.Fatal("expected field errors")
	}

	byField := map[string]validate.FieldError{}
	for _, e := range errs {
		byField[e.Field] = e
		// Every error must carry a readable message, so a form can show it.
		if e.Message == "" {
			t.Errorf("field %q has no message", e.Field)
		}
	}

	// Field names come from the JSON tags, and nested paths keep their index.
	if _, ok := byField["buyerName"]; !ok {
		t.Errorf("missing buyerName; got %v", byField)
	}
	if e, ok := byField["buyerEmail"]; !ok || e.Message != "must be a valid email address" {
		t.Errorf("buyerEmail = %+v", e)
	}
	if _, ok := byField["items[0].quantity"]; !ok {
		t.Errorf("missing items[0].quantity; got %v", byField)
	}
}

func TestStructAcceptsValidInput(t *testing.T) {
	v := validate.New()

	errs, ok := v.Struct(payload{
		BuyerName:  "Ada Lovelace",
		BuyerEmail: "ada@example.com",
		Items:      []item{{ProductID: "4aa6815b-280c-4913-9d1c-276a6d66263c", Quantity: 2}},
	})
	if !ok {
		t.Fatal("validation reported a non-field failure")
	}
	if len(errs) != 0 {
		t.Errorf("valid payload produced %+v", errs)
	}
}
