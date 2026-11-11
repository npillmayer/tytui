package fontbackend

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/npillmayer/fontfind"
)

// Origin is descriptive provenance, never a substitute for a content hash.
// Location may be relative to an embedded filesystem rather than an OS path.
type Origin struct {
	Provider, Name, Location string
	Request                  fontfind.Descriptor
}

// Resource owns an immutable byte snapshot. Copies may safely share its private data.
// Construct it before publishing Settings to an asynchronous pipeline command.
type Resource struct {
	data   []byte
	origin Origin
}

// ReadResource snapshots a bounded reader without parsing or resolving a font.
func ReadResource(src io.Reader, origin Origin) (*Resource, error) {
	if src == nil {
		return nil, fmt.Errorf("font reader is nil")
	}
	data, err := io.ReadAll(io.LimitReader(src, maxFontBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read font resource: %w", err)
	}
	if len(data) > maxFontBytes {
		return nil, fmt.Errorf("font exceeds 64 MiB")
	}
	return &Resource{data: data, origin: origin}, nil
}

// FontfindResource snapshots an already resolved offline resource; it performs no
// discovery or fallback. ReadFontData currently allocates before our size check:
// use ReadResource for bounded streams, and restrict this adapter to trusted resources.
func FontfindResource(sf fontfind.ScalableFont, provider string, request fontfind.Descriptor) (*Resource, error) {
	data, err := sf.ReadFontData()
	if err != nil {
		return nil, fmt.Errorf("font resource %q: %w", sf.Name, err)
	}
	return ReadResource(bytes.NewReader(data), Origin{Provider: provider, Name: sf.Name, Location: sf.Path(), Request: request})
}

func LoadResource(resource *Resource) (*Face, error) {
	if resource == nil {
		return nil, fmt.Errorf("nil font resource")
	}
	face, err := (Loader{}).LoadFont(bytes.NewReader(resource.data))
	if err != nil {
		return nil, err
	}
	f := face.(*Face)
	f.info.Origin = resource.origin
	f.info.Path = resource.origin.Location
	f.setResourceID()
	return f, nil
}

func (f *Face) setResourceID() {
	// Content identity is SHA256; ResourceID additionally distinguishes provenance.
	f.info.ResourceID = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(fmt.Sprintf("%q|%q|%q|%q|%d|%d|%s", f.info.Origin.Provider, f.info.Origin.Name, f.info.Path, f.info.Origin.Request.Pattern, f.info.Origin.Request.Style, f.info.Origin.Request.Weight, f.info.SHA256))))
}

// Open returns a fresh reader over immutable snapshot bytes without exposing them.
func (r *Resource) Open() io.ReadCloser {
	return io.NopCloser(struct{ io.Reader }{bytes.NewReader(r.data)})
}
