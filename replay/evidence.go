package replay

import (
	"context"
	"errors"
	"fmt"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"tytui/pipeline"
)

// CaptureRevisions records HEAD and tracked/untracked dirty status for the local
// development modules. Call at layout time for accurate provenance. Dirty state
// is an indication, not an archive of the actual patch or compiler/toolchain.
// Module directories without .git are recorded as unversioned and dirty.
func CaptureRevisions(ctx context.Context, workspace string) ([]Revision, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	var revisions []Revision
	for _, name := range []string{"tytui", "cords", "khipu", "uax", "opentype", "w3c-dom", "tree", "schuko", "fontfind", "typography"} {
		dir := workspace
		if name != "tytui" {
			dir = filepath.Join(workspace, "..", name)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); errors.Is(err, os.ErrNotExist) {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
				return nil, fmt.Errorf("revision %s: %w", name, err)
			}
			revisions = append(revisions, Revision{Module: name, Commit: "unversioned", Dirty: true})
			continue
		} else if err != nil {
			return nil, fmt.Errorf("revision %s: %w", name, err)
		}
		run := func(args ...string) (string, error) {
			cmd := exec.CommandContext(ctx, "git", args...)
			cmd.Dir = dir
			b, e := cmd.Output()
			return strings.TrimSpace(string(b)), e
		}
		commit, err := run("rev-parse", "HEAD")
		if err != nil {
			return nil, fmt.Errorf("revision %s: %w", name, err)
		}
		status, err := run("status", "--porcelain")
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, Revision{Module: name, Commit: commit, Dirty: status != ""})
	}
	return revisions, nil
}

type Run struct {
	Knot               int
	ResourceID, SHA256 string
	Replacement        bool
	ReplacementText    string
	EmSize             int32
	Direction          int
	Script, Language   string
	Features           []contracts.ShapeFeature
	Glyphs             []contracts.ShapedGlyph
	Clusters           []khipu.FrozenCluster
}

// LayoutEvidence is the version-3 numeric placement transcript, including for
// synthetic cases without glyph outlines. Versions 1/2 omit it entirely.
type LayoutEvidence struct {
	Width, Extent, Baseline float64
	HasBaseline             bool
	Lines                   []LinePlacement
}
type LinePlacement struct{ X, Top, Baseline, Ascent, Descent, Gap, Height float64 }

type Evidence struct {
	Layout *LayoutEvidence `json:",omitempty"`
	Lines  []khipu.FrozenLine
	Knots  []khipu.FrozenKnot
	Runs   []Run
	Proof  *pipeline.GlyphProof
}

func evidence(result *pipeline.Result) (Evidence, error) {
	var e Evidence
	if result.LineLayout() != nil {
		v := result.VList()
		if v == nil {
			return e, fmt.Errorf("line-height result lacks placements")
		}
		b := v.Box()
		e.Layout = &LayoutEvidence{Width: b.Width, Extent: b.Extent, Baseline: b.Baseline, HasBaseline: b.HasBaseline}
		for _, l := range v.Lines() {
			e.Layout.Lines = append(e.Layout.Lines, LinePlacement{l.X, l.Top, l.Baseline, l.Ascent, l.Descent, l.Gap, l.Height})
		}
	}
	for i := 0; i < result.Frozen.LineCount(); i++ {
		line, _ := result.Frozen.Line(i)
		e.Lines = append(e.Lines, line)
	}
	for i := 0; i < result.Frozen.KnotCount(); i++ {
		knot, _ := result.Frozen.Knot(i)
		e.Knots = append(e.Knots, knot)
		text, ok := result.Frozen.CopyText(i)
		if !ok {
			continue
		}
		run := Run{Knot: i, Replacement: text.IsReplacement, ReplacementText: text.ReplacementText, EmSize: int32(text.Context.EmSize), Direction: int(text.Context.Params.Direction), Script: text.Context.Params.Script.String(), Language: text.Context.Params.Language.String(), Features: text.Context.Params.Features, Glyphs: text.Glyphs, Clusters: text.Clusters}
		if result.Input.Fonts != nil {
			if result.Identity == nil {
				return e, fmt.Errorf("result lacks font identity service")
			}
			info, err := result.Identity.Identify(text.Context.Params.Font)
			if err != nil {
				return e, err
			}
			run.ResourceID, run.SHA256 = info.ResourceID, info.SHA256
		}
		e.Runs = append(e.Runs, run)
	}
	if result.Input.Fonts != nil {
		var err error
		e.Proof, err = pipeline.BuildGlyphProof(result)
		if err != nil {
			return e, err
		}
	}
	return e, nil
}
