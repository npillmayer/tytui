package main

import (
	"context"
	"errors"
	"github.com/npillmayer/khipu/dimen"
	"testing"
	"tytui/fontselection"
	"tytui/pipeline"
)

func fontJobModel(t *testing.T) model {
	t.Helper()
	m := initialModel()
	m.fontSettings = &fontselection.Request{Settings: fontselection.Settings{DefaultPath: uiFont, Size: 12 * dimen.BP}}
	return m
}

func TestFontJobCancellationDoesNotStartLayout(t *testing.T) {
	m := fontJobModel(t)
	m.run = func(pipeline.Input) (*pipeline.Result, error) {
		t.Fatal("cancelled font job started layout")
		return nil, nil
	}
	m, cmd := updateModel(m, runKey())
	m.invalidate()
	msg := cmd().(jobFontsMsg)
	if msg.selection != nil || !errors.Is(msg.err, context.Canceled) {
		t.Fatal(msg.err)
	}
	m, next := updateModel(m, msg)
	if next != nil || m.activeID != 0 || m.result != nil {
		t.Fatal("cancelled job published or scheduled layout")
	}
}

func TestFontHandoffRejectsStaleAndDuplicateCompletion(t *testing.T) {
	for _, stale := range []bool{false, true} {
		m := fontJobModel(t)
		calls := 0
		m.run = func(in pipeline.Input) (*pipeline.Result, error) { calls++; return pipeline.Run(in) }
		m, cmd := updateModel(m, runKey())
		msg := cmd().(jobFontsMsg)
		if msg.err != nil {
			t.Fatal(msg.err)
		}
		if stale {
			m.invalidate()
		}
		m, layout := updateModel(m, msg)
		if stale {
			if layout != nil || m.activeID != 0 || calls != 0 {
				t.Fatal("stale font selection reached layout")
			}
			continue
		}
		if layout == nil || m.cancelFonts != nil {
			t.Fatal("font handoff failed")
		}
		m, duplicate := updateModel(m, msg)
		if duplicate != nil {
			t.Fatal("duplicate font completion scheduled concurrent layout")
		}
		m = finishCommand(m, layout)
		if calls != 1 || m.result == nil || m.activeID != 0 {
			t.Fatal("layout did not complete exactly once")
		}
	}
}
