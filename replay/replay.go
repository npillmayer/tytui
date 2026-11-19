// Package replay stores versioned paragraph cases and exact fonts for offline replay.
// Font bytes live in a separate local content store, not inside the JSON manifest.
package replay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"

	"github.com/npillmayer/khipu/dimen"
	fontmeta "github.com/npillmayer/typography/font"
	"github.com/npillmayer/typography/fontresource"
	"github.com/npillmayer/typography/glyphing"
	otadapter "github.com/npillmayer/typography/glyphing/adapter/opentype"
	"github.com/npillmayer/typography/glyphing/paragraph"

	"tytui/fontselection"
	"tytui/pipeline"
)

// Version is the only supported experimental format. Historical formats are rejected.
const Version = 5
const maxBytes = 64 << 20

// Revision describes source provenance, not a promise of identical build output.
type Revision struct {
	Module, Commit string
	Dirty          bool
}

// Font and Selection describe pinned resources and selection provenance.
// Font wire records are independent of runtime identity and adapter types.
type Font struct {
	Info  wireInfo
	Bytes int64
}
type Rule struct {
	Family                           string
	Weight                           int
	Slant, ResourceID, RequestedPath string
}
type Selection struct {
	Policy                 *wirePolicy
	Default, RequestedPath string
	Rules                  []Rule
	Fallbacks              []string
	RequestedFallbackPaths []string
	Descriptors            *wireDescriptors
	Size                   int32
	Diagnostics            []wireDiagnostic
}
type Manifest struct {
	GoVersion      string
	Evidence       Evidence
	Version        int
	Units          string
	Input          wireInput // Fonts is always nil; Selection contains pinned references.
	Fonts          []Font
	Selection      *Selection
	Bindings       []wireBinding
	Revisions      []Revision
	Diagnostics    []pipeline.Diagnostic
	ParagraphText  string
	BreakPositions []uint64
}

func validHash(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}
func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
func readBounded(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBytes {
		return nil, fmt.Errorf("resource exceeds 64 MiB")
	}
	return data, nil
}

// Save writes a new manifest (never overwriting one) and pins fonts in store/hash.
// The store is local persistence; copying it for redistribution requires checking
// font licenses. Revisions are explicitly supplied by CaptureRevisions or a caller.
func Save(result *pipeline.Result, manifestPath, store string, revisions []Revision) error {
	if result == nil || result.Frozen == nil {
		return fmt.Errorf("save requires a context and completed result")
	}

	if len(revisions) == 0 {
		return fmt.Errorf("source revisions required; use CaptureRevisions at layout time")
	}

	m := Manifest{GoVersion: runtime.Version(), Version: Version, Units: "65536 DU per big point; 72 big points per inch", Input: inputWire(result.Input), Bindings: bindingsWire(result.FontBindings), Revisions: append([]Revision(nil), revisions...), Diagnostics: append([]pipeline.Diagnostic(nil), result.Diagnostics...), ParagraphText: result.ParagraphText, BreakPositions: append([]uint64(nil), result.BreakPositions...)}
	m.Input.LineLayout = result.LineLayout()
	m.Input.Assembly = paragraph.CSSLineHeight
	if m.Input.LineLayout == nil {
		return fmt.Errorf("result lacks CSS line-height; rerun layout")
	}
	var evidenceErr error
	m.Evidence, evidenceErr = evidence(result)
	if evidenceErr != nil {
		return evidenceErr
	}
	m.Input.Fonts = nil
	m.Input.Source = result.Input.Source.Clone()
	if result.Input.Fonts != nil {
		if result.FontSnapshot == nil {
			return fmt.Errorf("result lacks pinned fonts; rerun layout")
		}
		settings := result.FontSnapshot
		provenance := result.FontProvenance()
		sel := &Selection{Policy: policyWire(provenance.DiscoveryPolicy), Size: int32(settings.Size), RequestedPath: provenance.RequestedPath, RequestedFallbackPaths: append([]string(nil), provenance.RequestedFallbackPaths...), Descriptors: descriptorsWire(provenance.DescriptorInput), Diagnostics: diagnosticsWire(provenance.ResolutionDiagnostics)}
		resources := map[string]*fontresource.Resource{}
		for _, face := range settings.Faces {
			resources[face.Info.ResourceID] = face.Resource
		}

		m.Selection = sel
		seen := map[string]bool{}
		pin := func(r *fontresource.Resource) (string, error) {
			if r == nil {
				return "", fmt.Errorf("missing pinned resource")
			}
			loaded, err := (otadapter.Services{}).Load(context.Background(), "", r)
			info := loaded.Info
			if err != nil {
				return "", err
			}
			if seen[info.ResourceID] {
				return info.ResourceID, nil
			}
			data, err := readFontSource(r)
			if err != nil {
				return "", err
			}
			if digest(data) != info.SHA256 {
				return "", fmt.Errorf("snapshot hash mismatch")
			}
			if err := put(store, info.SHA256, data); err != nil {
				return "", err
			}
			seen[info.ResourceID] = true
			m.Fonts = append(m.Fonts, Font{Info: infoWire(info), Bytes: int64(len(data))})
			return info.ResourceID, nil
		}
		var err error
		sel.Default, err = pin(resources[settings.Default])
		if err != nil {
			return err
		}
		for i, rule := range settings.Rules {
			id, err := pin(resources[rule.ResourceID])
			if err != nil {
				return err
			}
			sel.Rules = append(sel.Rules, Rule{Family: rule.Family, Weight: rule.Weight, Slant: rule.Slant, ResourceID: id, RequestedPath: requestedRulePath(provenance, i)})
		}
		for _, resource := range settings.Fallbacks {
			id, err := pin(resources[resource])
			if err != nil {
				return err
			}
			sel.Fallbacks = append(sel.Fallbacks, id)
		}
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxBytes {
		return fmt.Errorf("manifest exceeds 64 MiB")
	}
	return writeNew(manifestPath, data)
}
func writeNew(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".case-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Atomic publication without replacing an existing manifest or store object.
	return os.Link(f.Name(), path)
}

func put(store, hash string, data []byte) error {
	if err := os.MkdirAll(store, 0700); err != nil {
		return err
	}
	path := filepath.Join(store, hash)
	err := writeNew(path, data)
	if os.IsExist(err) {
		existing, readErr := readFile(path)
		if readErr != nil {
			return readErr
		}
		if digest(existing) != hash {
			return fmt.Errorf("corrupt font store object %s", hash)
		}
		return nil
	}
	return err
}
func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readBounded(f)
}

// Load checks schema, references, sizes and hashes before invoking the normal
// pipeline. It never resolves descriptors, opens original paths, or fetches fonts.
// Changes in layout behavior are reported rather than silently accepted.
func Load(manifestPath, store string) (*pipeline.Result, error) {
	data, err := readFile(manifestPath)
	if err != nil {
		return nil, err
	}
	// Inspect only the version before decoding the current schema. Historical
	// fields must produce a version error, never invoke an old reader or font I/O.
	var header struct{ Version int }
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	if header.Version != Version {
		return nil, fmt.Errorf("unsupported replay version %d; current experimental version is %d", header.Version, Version)
	}
	var m Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("trailing manifest data")
	}
	if m.Units != "65536 DU per big point; 72 big points per inch" {
		return nil, fmt.Errorf("unsupported replay units")
	}
	switch m.Input.Assembly {
	case paragraph.CSSLineHeight:
		if m.Input.LineLayout == nil {
			return nil, fmt.Errorf("CSS assembly requires line-height values")
		}
	default:
		return nil, fmt.Errorf("unknown or missing assembly mode %q", m.Input.Assembly)
	}
	if m.Evidence.Layout == nil {
		return nil, fmt.Errorf("replay requires placement evidence")
	}
	if m.Input.hasFonts() {
		return nil, fmt.Errorf("manifest contains unpinned font settings")
	}
	if m.Input.Source != nil && m.Input.Source.OriginalHTML != "" && digest([]byte(m.Input.Source.OriginalHTML)) != m.Input.Source.DocumentID {
		return nil, fmt.Errorf("HTML source hash mismatch")
	}
	input := m.Input.native()
	resources := map[string]glyphing.FaceEntry{}
	for _, entry := range m.Fonts {
		if !validHash(entry.Info.SHA256) || entry.Bytes < 0 || entry.Bytes > maxBytes {
			return nil, fmt.Errorf("invalid font hash or size")
		}
		if _, ok := resources[entry.Info.ResourceID]; ok {
			return nil, fmt.Errorf("duplicate resource ID")
		}
		data, err := readFile(filepath.Join(store, entry.Info.SHA256))
		if err != nil {
			return nil, fmt.Errorf("required font %s unavailable: %w", entry.Info.SHA256, err)
		}
		if int64(len(data)) != entry.Bytes || digest(data) != entry.Info.SHA256 {
			return nil, fmt.Errorf("font %s hash/size mismatch", entry.Info.SHA256)
		}
		resource, err := fontresource.Read(bytes.NewReader(data), entry.Info.Origin.native())
		if err != nil {
			return nil, err
		}
		loaded, err := (otadapter.Services{}).Load(context.Background(), "", resource)
		info := loaded.Info
		if err != nil {
			return nil, err
		}
		if info.ResourceID != entry.Info.ResourceID || info.Family != entry.Info.Family || info.Subfamily != entry.Info.Subfamily || info.Path != entry.Info.Path {
			return nil, fmt.Errorf("font identity mismatch for %s", entry.Info.SHA256)
		}
		resources[entry.Info.ResourceID] = loaded
	}
	if m.Selection != nil {
		sel := m.Selection
		s := &fontselection.Selection{Fonts: glyphing.SelectedFonts{Default: sel.Default, Size: dimen.DU(sel.Size)}, Provenance: fontselection.Provenance{RequestedPath: sel.RequestedPath, RequestedFallbackPaths: append([]string(nil), sel.RequestedFallbackPaths...), DiscoveryPolicy: sel.Policy.native(), DescriptorInput: sel.Descriptors.native(), ResolutionDiagnostics: diagnosticsNative(sel.Diagnostics)}}
		seen := map[string]bool{}
		use := func(id string) error {
			entry, ok := resources[id]
			if !ok {
				return fmt.Errorf("unresolved font reference %q", id)
			}
			if !seen[id] {
				seen[id] = true
				s.Fonts.Faces = append(s.Fonts.Faces, entry)
			}
			return nil
		}
		if err := use(sel.Default); err != nil {
			return nil, err
		}
		for _, rule := range sel.Rules {
			if err := use(rule.ResourceID); err != nil {
				return nil, err
			}
			s.Provenance.RequestedRulePaths = append(s.Provenance.RequestedRulePaths, rule.RequestedPath)
			s.Fonts.Rules = append(s.Fonts.Rules, glyphing.SelectedRule{Family: rule.Family, Weight: rule.Weight, Slant: rule.Slant, ResourceID: rule.ResourceID})
		}
		for _, id := range sel.Fallbacks {
			if err := use(id); err != nil {
				return nil, err
			}
			s.Fonts.Fallbacks = append(s.Fonts.Fallbacks, id)
		}
		input.Fonts = s
	} else if len(m.Fonts) > 0 || len(m.Bindings) > 0 {
		return nil, fmt.Errorf("fonts without selection")
	}
	result, err := pipeline.Run(input)
	if err != nil {
		return nil, err
	}
	if result.ParagraphText != m.ParagraphText || !reflect.DeepEqual(result.BreakPositions, m.BreakPositions) || !reflect.DeepEqual(bindingsWire(result.FontBindings), m.Bindings) {
		return nil, fmt.Errorf("replay differs from recorded paragraph, breaks, or font bindings")
	}
	actual, err := evidence(result)
	if err != nil {
		return nil, err
	}
	for _, check := range []struct {
		name             string
		actual, recorded any
	}{
		{"placements", actual.Layout, m.Evidence.Layout},
		{"frozen lines", actual.Lines, m.Evidence.Lines},
		{"frozen knots", actual.Knots, m.Evidence.Knots},
		{"shaped runs", actual.Runs, m.Evidence.Runs},
		{"glyph proof", actual.Proof, m.Evidence.Proof},
	} {
		if !reflect.DeepEqual(check.actual, check.recorded) {
			return nil, fmt.Errorf("replay differs from recorded %s", check.name)
		}
	}
	return result, nil
}

func readFontSource(source fontmeta.Source) ([]byte, error) {
	reader := source.Open()
	defer reader.Close()
	return readBounded(reader)
}

func requestedRulePath(p fontselection.Provenance, i int) string {
	if i < len(p.RequestedRulePaths) {
		return p.RequestedRulePaths[i]
	}
	return ""
}
