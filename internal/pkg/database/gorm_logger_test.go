package database

import (
	"bytes"
	"context"
	"testing"
)

type gormParamsFilter interface {
	ParamsFilter(ctx context.Context, sql string, params ...interface{}) (string, []interface{})
}

func TestParameterizedGORMLoggerDropsQueryParameters(t *testing.T) {
	var output bytes.Buffer
	databaseLogger := newParameterizedGORMLogger(&output)
	filter, ok := databaseLogger.(gormParamsFilter)
	if !ok {
		t.Fatal("GORM logger does not expose parameter filtering")
	}

	const query = "SELECT * FROM product_reservations WHERE contact_hash = ?"
	filteredQuery, filteredParams := filter.ParamsFilter(t.Context(), query, "sensitive-contact-hash")
	if filteredQuery != query {
		t.Fatalf("filtered query = %q", filteredQuery)
	}
	if filteredParams != nil {
		t.Fatalf("parameterized logger retained parameters: %#v", filteredParams)
	}
	if output.String() != "" {
		t.Fatalf("parameter filtering unexpectedly logged data: %q", output.String())
	}
}
