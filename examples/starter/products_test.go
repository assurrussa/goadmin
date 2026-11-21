package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/assurrussa/goadmin/features/crud"
	"github.com/assurrussa/goadmin/host"
)

func TestUnpublishRejectsInvalidReasonBeforeOpeningTransaction(t *testing.T) {
	for _, reason := range []string{"", " \u0085\n ", strings.Repeat("🙂", 501)} {
		err := (productRepo{}).unpublish(t.Context(), host.Actor{AdminID: 1}, "1", "1", UnpublishInput{Reason: reason})
		var fields crud.FieldErrors
		if !errors.As(err, &fields) || len(fields["reason"]) != 1 {
			t.Fatalf("expected one reason field error, got %v", err)
		}
	}
}

func TestValidateProduct(t *testing.T) {
	name := strings.Repeat("🙂", 200)
	for _, test := range []struct {
		label string
		input productInput
		want  crud.FieldErrors
	}{
		{"inclusive bounds", productInput{Name: name, Stock: 0}, nil},
		{"largest safe stock", productInput{Name: name, Stock: 9007199254740991}, nil},
		{"both field constraints", productInput{Name: name + "🙂", Stock: -1}, crud.FieldErrors{
			"name": {"Text is too long"}, "stock": {"Value is below the minimum"},
		}},
		{"blank name and unsafe stock", productInput{Name: "   ", Stock: 9007199254740992}, crud.FieldErrors{
			"name": {"Name cannot be blank"}, "stock": {"number exceeds the safe range"},
		}},
		{"text PostgreSQL cannot store", productInput{Name: "Part\x00 A", Description: "details\x00"}, crud.FieldErrors{
			"name": {"Invalid field value"}, "description": {"Invalid field value"},
		}},
		{"invalid UTF-8 from Go", productInput{Name: name, Description: "\xff"}, crud.FieldErrors{
			"description": {"Invalid field value"},
		}},
		{"date outside the form contract", productInput{Name: name, Available: "0000-01-01"}, crud.FieldErrors{
			"available": {"Use YYYY-MM-DD"},
		}},
	} {
		t.Run(test.label, func(t *testing.T) {
			err := validateProduct(test.input)
			var fields crud.FieldErrors
			if err != nil && !errors.As(err, &fields) {
				t.Fatalf("expected field errors, got %v", err)
			}
			if !reflect.DeepEqual(fields, test.want) {
				t.Fatalf("errors = %#v, want %#v", fields, test.want)
			}
		})
	}
}
