package documentlayout_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/npillmayer/typography/glyphing/paragraph"

	"tytui/documentlayout"
)

func TestParagraphDisplayMatchesExplicitBlock(t *testing.T) {
	for _, source := range []string{
		`<p id="root">Café <em>styled</em> text.</p>`,
		`<div id="root"><p>Café <em>styled</em> text.</p><p dir="rtl" lang="he">אב גד</p></div>`,
	} {
		implicit, input, _ := build(t, source)
		explicit, _, _ := build(t, strings.ReplaceAll(source, "<p", "<p style=\"display:block\""))
		for _, node := range input.Nodes {
			if node.Name != "p" {
				continue
			}
			v, err := input.Property(node.ID, "display")
			if err != nil || v != "block" {
				t.Fatalf("paragraph display = %q, %v; want block", v, err)
			}
		}
		for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
			got := runDocument(t, implicit, 100, breaker, layoutConfig(t))
			want := runDocument(t, explicit, 100, breaker, layoutConfig(t))
			if !reflect.DeepEqual(got.Geometry(), want.Geometry()) {
				t.Fatal("default and explicit block geometry differ")
			}
			if len(got.Groups()) != len(want.Groups()) {
				t.Fatal("inline group count differs")
			}
			for _, group := range got.Groups() {
				g, _ := got.Group(group.GroupID)
				w, ok := want.Group(group.GroupID)
				if !ok || g.Input.Text != w.Input.Text || !reflect.DeepEqual(g.Paragraph.BreakPositions, w.Paragraph.BreakPositions) {
					t.Fatal("paragraph text/breaks differ")
				}
			}
			var svg bytes.Buffer
			if err := documentlayout.WriteBoxesSVG(&svg, got); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestParagraphDisplayOverrideStillApplies(t *testing.T) {
	for _, display := range []string{"inline", "inline-block", "none", "flex"} {
		input, id := parse(t, `<p id="root" style="display:`+display+`">text</p>`)
		if _, err := documentlayout.Build(input, id); err == nil {
			t.Fatalf("accepted non-block paragraph root: %s", display)
		}
	}
	d, _, _ := build(t, `<div id="root">before<p style="display:inline">middle</p>after</div>`)
	groups := extractAll(t, d)
	if len(groups) != 1 || groups[0].Text != "beforemiddleafter" {
		t.Fatalf("inline override lost: %+v", groups)
	}
}
