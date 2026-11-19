package documentlayout_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/npillmayer/typography/glyphing/paragraph"

	"tytui/documentlayout"
	"tytui/htmlinput"
)

func imageFixture(t *testing.T, format string) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 128})
	var b bytes.Buffer
	var err error
	if format == "png" {
		err = png.Encode(&b, img)
	} else {
		err = jpeg.Encode(&b, img, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func imageDocument(t *testing.T, name, source string) (*htmlinput.Document, int) {
	t.Helper()
	d, err := htmlinput.Parse(name, strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range d.Nodes {
		for _, a := range n.Attributes {
			if a.Key == "id" && a.Val == "root" {
				return d, n.ID
			}
		}
	}
	t.Fatal("missing root")
	return nil, 0
}
func TestLocalImageRetentionAndGeometry(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		for _, dir := range []string{"ltr", "rtl"} {
			for _, breaker := range []paragraph.Breaker{paragraph.FirstFit, paragraph.KnuthPlass} {
				t.Run(format+dir+string(breaker), func(t *testing.T) {
					directory := t.TempDir()
					path := filepath.Join(directory, "local image.bin")
					data := imageFixture(t, format)
					if err := os.WriteFile(path, data, 0600); err != nil {
						t.Fatal(err)
					}
					input, id := imageDocument(t, filepath.Join(directory, "source.html"), `<div id="root" dir="`+dir+`" style="font-size:20px">é <span style="display:inline-block"><img data-tytui-leaf="image" src="local%20image.bin" alt="A &amp; B" style="width:30px;height:12px;padding:2px;border-width:1px;border-style:solid;margin:3px"></span> אב<img data-tytui-leaf="image" src="local%20image.bin" style="width:0;height:0"></div>`)
					d, err := documentlayout.Build(input, id)
					if err != nil {
						t.Fatal(err)
					}
					if len(d.Groups()) != 2 {
						t.Fatal("image got paragraph interior")
					}
					checkSource(t, d, extractAll(t, d))
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					} // layout itself must use retained bytes
					r := runDocument(t, d, 120, breaker, layoutConfig(t))
					p, err := documentlayout.BuildProof(r, cells())
					if err != nil {
						t.Fatal(err)
					}
					if len(p.Leaves) != 2 || p.Leaves[0].Image != p.Leaves[1].Image {
						t.Fatal("occurrences did not share one immutable byte snapshot")
					}
					for _, leaf := range p.Leaves {
						info := leaf.Image.Info()
						if info.Path != path || info.MIME != "image/"+format || info.Width != 3 || info.Height != 2 || info.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) {
							t.Fatal(info)
						}
						if !bytes.Equal(leaf.Image.Bytes(), data) {
							t.Fatal("resource bytes changed")
						}
						copy := leaf.Image.Bytes()
						copy[0] ^= 255
						info.SHA256 = "modified"
						if leaf.Label == "A & B" {
							if leaf.Content.W != 22.5 || leaf.Content.H != 9 || r.Geometry()[leaf.BoxID].Frame.W != 36 || r.Geometry()[leaf.BoxID].Frame.H != 18 {
								t.Fatal("pixel dimensions leaked into CSS geometry", leaf)
							}
						} else if leaf.Content.W != 0 || leaf.Content.H != 0 {
							t.Fatal("zero image size changed")
						}
						found := false
						for _, a := range p.Atomics {
							if a.BoxID == leaf.BoxID {
								found = true
								if a.Locator != leaf.Locator {
									t.Fatal("source identity lost")
								}
							}
						}
						if !found {
							t.Fatal("image detached from atomic")
						}
					}
					var before, after bytes.Buffer
					if err := documentlayout.WriteSVG(&before, r, cells()); err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(before.String(), "data:image/"+format+";base64,"+base64.StdEncoding.EncodeToString(data)) || !strings.Contains(before.String(), `preserveAspectRatio="none"`) {
						t.Fatal("SVG did not embed exact retained payload")
					}
					dec := xml.NewDecoder(bytes.NewReader(before.Bytes()))
					for {
						_, err := dec.Token()
						if err == io.EOF {
							break
						}
						if err != nil {
							t.Fatal(err)
						}
					}
					p.Leaves[0].Image = nil
					p.Leaves[0].Label = "changed"
					runDocument(t, d, 65, breaker, layoutConfig(t))
					if next, err := documentlayout.Build(input, id); err == nil || next != nil {
						t.Fatal("missing image did not fail new build")
					}
					if err := documentlayout.WriteSVG(&after, r, cells()); err != nil || !bytes.Equal(before.Bytes(), after.Bytes()) {
						t.Fatal("retained proof changed", err)
					}
				})
			}
		}
	}
}
func TestLocalImageFailures(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "bad.png"), []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	data := imageFixture(t, "png")
	if err := os.WriteFile(filepath.Join(directory, "truncated.png"), data[:len(data)/2], 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(directory, "large.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate((8 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, src := range []string{"", "https://example.org/image.png", "//example.org/image.png", "data:image/png;base64,abc", "file:///tmp/image.png", "bad.png", "truncated.png", "large.png", "missing.png", ".", "bad.png?query", "bad.png#fragment"} {
		input, id := imageDocument(t, filepath.Join(directory, "source.html"), `<div id="root"><img data-tytui-leaf="image" src="`+src+`" style="width:20px;height:10px"></div>`)
		if d, err := documentlayout.Build(input, id); err == nil || d != nil {
			t.Fatal("accepted image", src)
		} else if !strings.Contains(err.Error(), "img") {
			t.Fatal("failure lacks source locator", err)
		}
	}
	input, id := imageDocument(t, filepath.Join(directory, "source.html"), `<div id="root"><img data-tytui-leaf="image" src="missing.png" style="width:20px;height:10px"></div>`)
	if _, err := input.Extract(id); err == nil {
		t.Fatal("paragraph mode accepted image")
	}
}

func TestLocalImagePixelLimitAndUnsupportedCodec(t *testing.T) {
	directory := t.TempDir()
	huge := imageFixture(t, "png")
	binary.BigEndian.PutUint32(huge[16:20], 1<<24)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	var gifData bytes.Buffer
	if err := gif.Encode(&gifData, image.NewNRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"huge.png", huge, "pixels"}, {"unsupported.gif", gifData.Bytes(), "only PNG and JPEG"},
	} {
		if err := os.WriteFile(filepath.Join(directory, tc.name), tc.data, 0600); err != nil {
			t.Fatal(err)
		}
		input, id := imageDocument(t, filepath.Join(directory, "source.html"), `<div id="root"><img data-tytui-leaf="image" src="`+tc.name+`" style="width:0;height:0"></div>`)
		if d, err := documentlayout.Build(input, id); d != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatal("invalid payload admitted", tc.name, err)
		}
	}
}
