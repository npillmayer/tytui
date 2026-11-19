package documentlayout

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const maxImageBytes = 8 << 20
const maxImagePixels = 16 << 20

// ImageInfo identifies the exact retained file bytes. Pixel dimensions are
// diagnostic metadata; CSS dimensions alone determine layout.
type ImageInfo struct {
	Path, MIME, SHA256 string
	Width, Height      int
}

// ImageResource owns validated immutable PNG/JPEG bytes. Accessors detach values;
// proofs may share this resource indefinitely, without reopening the source.
type ImageResource struct {
	info ImageInfo
	data []byte
}

func (r *ImageResource) Info() ImageInfo { return r.info }
func (r *ImageResource) Bytes() []byte   { return slices.Clone(r.data) }

// LeafPayload is renderer data; neither CSS core nor paragraph layout sees it.
// Kind is rectangle or image. Image is immutable; copying this value is safe.
type LeafPayload struct {
	Kind, Label string
	Image       *ImageResource
}

func localImagePath(documentName, src string) (string, error) {
	u, err := url.Parse(src)
	if err != nil || strings.TrimSpace(src) == "" || u.Scheme != "" || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || strings.HasPrefix(src, "//") || strings.ContainsAny(src, "?#") {
		return "", unsupported("image src must be a local path without scheme, host, query or fragment")
	}
	path := u.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(documentName), path)
	}
	return filepath.Abs(path)
}

func readImage(path string) (*ImageResource, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > maxImageBytes {
		return nil, fmt.Errorf("image must be a regular file of at most %d bytes", maxImageBytes)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("image exceeds %d bytes", maxImageBytes)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image header: %w", err)
	}
	if format != "png" && format != "jpeg" {
		return nil, unsupported("only PNG and JPEG images are supported")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxImagePixels {
		return nil, fmt.Errorf("image exceeds %d pixels", maxImagePixels)
	}
	// Decode the complete payload now, so damaged files cannot become successful
	// layouts whose proofs fail later. The pixel buffer is not retained.
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("image decode: %w", err)
	}

	return &ImageResource{data: data, info: ImageInfo{Path: path, MIME: "image/" + format, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Width: config.Width, Height: config.Height}}, nil
}
