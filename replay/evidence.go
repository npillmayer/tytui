package replay

import (
	"context"
	"fmt"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/contracts"
	"os/exec"
	"path/filepath"
	"strings"
	"tytui/fontbackend"
	"tytui/pipeline"
)

// CaptureRevisions records HEAD and tracked/untracked dirty status for the local
// development modules. Call at layout time for accurate provenance. Dirty state
// is an indication, not an archive of the actual patch or compiler/toolchain.
func CaptureRevisions(ctx context.Context, workspace string) ([]Revision, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	var revisions []Revision
	for _, name := range []string{"tytui", "cords", "khipu", "uax", "opentype", "w3c-dom", "tree", "schuko", "fontfind"} {
		dir := workspace
		if name != "tytui" {
			dir = filepath.Join(workspace, "..", name)
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
type Evidence struct {
	Lines []khipu.FrozenLine
	Knots []khipu.FrozenKnot
	Runs  []Run
	Proof *pipeline.GlyphProof
}

func evidence(result *pipeline.Result) (Evidence, error) {
	var e Evidence
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
		if face, ok := text.Context.Params.Font.(*fontbackend.Face); ok {
			info := face.Info()
			run.ResourceID = info.ResourceID
			run.SHA256 = info.SHA256
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
