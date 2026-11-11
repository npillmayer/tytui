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
	"github.com/npillmayer/khipu/dimen"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"

	"tytui/fontbackend"
	"tytui/pipeline"
)

const Version = 1
const maxBytes = 64 << 20

// Revision describes source provenance, not a promise of identical build output.
type Revision struct {
	Module, Commit string
	Dirty          bool
}
type Font struct {
	Info  fontbackend.Info
	Bytes int64
}
type Rule struct {
	Family                           string
	Weight                           int
	Slant, ResourceID, RequestedPath string
}
type Selection struct {
	Policy                 *fontbackend.DiscoveryPolicy
	Default, RequestedPath string
	Rules                  []Rule
	Fallbacks              []string
	RequestedFallbackPaths []string
	Descriptors            *fontbackend.DescriptorSettings
	Size                   int32
	Diagnostics            []fontbackend.FontDiagnostic
}
type Manifest struct {
	GoVersion      string
	Evidence       Evidence
	Version        int
	Units          string
	Input          pipeline.Input // Fonts is always nil; Selection contains pinned references.
	Fonts          []Font
	Selection      *Selection
	Bindings       []fontbackend.Binding
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
func Save(ctx context.Context, result *pipeline.Result, manifestPath, store string, revisions []Revision) error {
	if ctx == nil || result == nil || result.Frozen == nil {
		return fmt.Errorf("save requires a context and completed result")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(revisions) == 0 {
		return fmt.Errorf("source revisions required; use CaptureRevisions at layout time")
	}
	m := Manifest{GoVersion: runtime.Version(), Version: Version, Units: "65536 DU per big point; 72 big points per inch", Input: result.Input, Bindings: append([]fontbackend.Binding(nil), result.FontBindings...), Revisions: append([]Revision(nil), revisions...), Diagnostics: append([]pipeline.Diagnostic(nil), result.Diagnostics...), ParagraphText: result.ParagraphText, BreakPositions: append([]uint64(nil), result.BreakPositions...)}
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
		sel := &Selection{Policy: settings.DiscoveryPolicy.Clone(), Size: int32(settings.Size), RequestedPath: result.Input.Fonts.DefaultPath, RequestedFallbackPaths: append([]string(nil), result.Input.Fonts.Fallbacks...), Descriptors: settings.DescriptorInput.Clone(), Diagnostics: append([]fontbackend.FontDiagnostic(nil), settings.ResolutionDiagnostics...)}
		m.Selection = sel
		seen := map[string]bool{}
		pin := func(r *fontbackend.Resource) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if r == nil {
				return "", fmt.Errorf("missing pinned resource")
			}
			face, err := fontbackend.LoadResource(r)
			if err != nil {
				return "", err
			}
			info := face.Info()
			if seen[info.ResourceID] {
				return info.ResourceID, nil
			}
			reader := r.Open()
			data, err := readBounded(reader)
			reader.Close()
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
			m.Fonts = append(m.Fonts, Font{Info: info, Bytes: int64(len(data))})
			return info.ResourceID, nil
		}
		var err error
		sel.Default, err = pin(settings.DefaultResource)
		if err != nil {
			return err
		}
		for i, rule := range settings.Rules {
			id, err := pin(rule.Resource)
			if err != nil {
				return err
			}
			sel.Rules = append(sel.Rules, Rule{Family: rule.Family, Weight: rule.Weight, Slant: rule.Slant, ResourceID: id, RequestedPath: result.Input.Fonts.Rules[i].Path})
		}
		for _, resource := range settings.FallbackResources {
			id, err := pin(resource)
			if err != nil {
				return err
			}
			sel.Fallbacks = append(sel.Fallbacks, id)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
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
func Load(ctx context.Context, manifestPath, store string) (*pipeline.Result, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := readFile(manifestPath)
	if err != nil {
		return nil, err
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
	if m.Version != Version || m.Units != "65536 DU per big point; 72 big points per inch" {
		return nil, fmt.Errorf("unsupported manifest version or units")
	}
	if m.Input.Fonts != nil {
		return nil, fmt.Errorf("manifest contains unpinned font settings")
	}
	if m.Input.Source != nil && m.Input.Source.OriginalHTML != "" && digest([]byte(m.Input.Source.OriginalHTML)) != m.Input.Source.DocumentID {
		return nil, fmt.Errorf("HTML source hash mismatch")
	}
	resources := map[string]*fontbackend.Resource{}
	for _, entry := range m.Fonts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
		resource, err := fontbackend.ReadResource(bytes.NewReader(data), entry.Info.Origin)
		if err != nil {
			return nil, err
		}
		face, err := fontbackend.LoadResource(resource)
		if err != nil {
			return nil, err
		}
		if face.Info().ResourceID != entry.Info.ResourceID || face.Info().Family != entry.Info.Family || face.Info().Subfamily != entry.Info.Subfamily || face.Info().Path != entry.Info.Path {
			return nil, fmt.Errorf("font identity mismatch for %s", entry.Info.SHA256)
		}
		resources[entry.Info.ResourceID] = resource
	}
	if m.Selection != nil {
		sel := m.Selection
		get := func(id string) (*fontbackend.Resource, error) {
			r, ok := resources[id]
			if !ok {
				return nil, fmt.Errorf("unresolved font reference %q", id)
			}
			return r, nil
		}
		s := &fontbackend.Settings{DiscoveryPolicy: sel.Policy.Clone(), Size: dimen.DU(sel.Size), DescriptorInput: sel.Descriptors.Clone(), ResolutionDiagnostics: append([]fontbackend.FontDiagnostic(nil), sel.Diagnostics...)}
		s.DefaultResource, err = get(sel.Default)
		if err != nil {
			return nil, err
		}
		for _, rule := range sel.Rules {
			r, err := get(rule.ResourceID)
			if err != nil {
				return nil, err
			}
			s.Rules = append(s.Rules, fontbackend.Rule{Family: rule.Family, Weight: rule.Weight, Slant: rule.Slant, Resource: r})
		}
		for _, id := range sel.Fallbacks {
			r, err := get(id)
			if err != nil {
				return nil, err
			}
			s.FallbackResources = append(s.FallbackResources, r)
		}
		m.Input.Fonts = s
	} else if len(m.Fonts) > 0 || len(m.Bindings) > 0 {
		return nil, fmt.Errorf("fonts without selection")
	}
	result, err := pipeline.Run(ctx, m.Input)
	if err != nil {
		return nil, err
	}
	if result.ParagraphText != m.ParagraphText || !reflect.DeepEqual(result.BreakPositions, m.BreakPositions) || !reflect.DeepEqual(result.FontBindings, m.Bindings) {
		return nil, fmt.Errorf("replay differs from recorded paragraph, breaks, or font bindings")
	}
	actual, err := evidence(result)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(actual, m.Evidence) {
		return nil, fmt.Errorf("replay differs from recorded frozen layout or glyph proof")
	}
	return result, nil
}
