// Package validate configures the DTO validator used at the HTTP boundary and
// turns its errors into the client-safe details the error envelope carries.
package validate

import (
	"errors"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Validator validates request DTOs.
type Validator struct {
	v *validator.Validate
}

// New returns a Validator that reports field names as they appear in JSON, so
// validation details match the payload the client actually sent.
func New() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())

	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return field.Name
		}
		return name
	})

	return &Validator{v: v}
}

// FieldError is one field-level validation failure.
type FieldError struct {
	Field string `json:"field"`
	Rule  string `json:"rule"`
}

// Struct validates s. It returns nil when valid, otherwise the field errors to
// place in the error envelope's details. The second return value is false when
// validation failed for a reason that is not field-level (a programming error,
// such as passing a non-struct), which callers should treat as a 500.
func (val *Validator) Struct(s any) (fieldErrs []FieldError, ok bool) {
	err := val.v.Struct(s)
	if err == nil {
		return nil, true
	}

	var invalid *validator.InvalidValidationError
	if errors.As(err, &invalid) {
		return nil, false
	}

	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		fieldErrs = make([]FieldError, 0, len(verrs))
		for _, fe := range verrs {
			fieldErrs = append(fieldErrs, FieldError{Field: fieldPath(fe), Rule: fe.Tag()})
		}
		return fieldErrs, true
	}
	return nil, false
}

// fieldPath renders the JSON path of a failing field relative to the DTO root,
// for example "items[0].quantity".
func fieldPath(fe validator.FieldError) string {
	ns := fe.Namespace()
	if i := strings.Index(ns, "."); i >= 0 {
		return ns[i+1:]
	}
	return fe.Field()
}
