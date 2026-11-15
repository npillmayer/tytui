package pipeline_test

import (
	"context"
	"testing"

	"tytui/fontselection"
)

func selectFonts(t testing.TB, request *fontselection.Request) *fontselection.Selection {
	t.Helper()
	s, err := fontselection.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
