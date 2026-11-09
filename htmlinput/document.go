// Package htmlinput loads a styled HTML DOM and extracts paragraph-like subtrees.
// Extraction resolves a CSS subset and retains transformed byte provenance.
package htmlinput

import (
	"crypto/sha256"
	"fmt"
	"github.com/npillmayer/uax/bidi"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/andybalholm/cascadia"
	dcss "github.com/aymerick/douceur/css"
	"github.com/aymerick/douceur/parser"
	dom "github.com/npillmayer/w3c-dom"
	"github.com/npillmayer/w3c-dom/style"
	"github.com/npillmayer/w3c-dom/style/cssom"
	"github.com/npillmayer/w3c-dom/style/cssom/douceuradapter"
	"github.com/npillmayer/w3c-dom/styledtree"
	"golang.org/x/net/html"
)

// Node identifies a DOM node in document preorder. IDs are document-local.
// Path counts all siblings, including text nodes, and is a locator, not XPath.
// Treat a published Document and its nodes as read-only.
type Node struct {
	ID, Parent, Depth int
	Path, Name, Text  string
	Type              html.NodeType
	Attributes        []html.Attribute
	Children          []int
	styled            *styledtree.StyNode
}

type Document struct {
	Name, ID string // ID is the SHA-256 of the original HTML bytes.
	Nodes    []Node
	Warnings []string
}

// Span maps extracted UTF-8 bytes to decoded bytes in one DOM text node.
// These are not offsets into the original HTML (entities and tags change them).
type Span struct {
	Start, End         uint64
	NodeID             int
	NodePath           string
	NodeStart, NodeEnd uint64
	Kind               string // Empty: identity; collapsed: many-to-one; omitted: zero output bytes.
}

// Extraction is detached provenance suitable for retaining with a frozen result.
type Extraction struct {
	DocumentName, DocumentID string
	SelectedID               int
	SelectedPath             string
	Text                     string
	Spans                    []Span
	Styles                   []StyleSpan
	Direction                bidi.Direction
	Isolates                 []Isolate
}

func (e *Extraction) Clone() *Extraction {
	if e == nil {
		return nil
	}
	copy := *e
	copy.Spans = append([]Span(nil), e.Spans...)
	copy.Styles = append([]StyleSpan(nil), e.Styles...)
	copy.Isolates = append([]Isolate(nil), e.Isolates...)
	return &copy
}

// LoadFile loads only this local file; linked stylesheets and resources are not fetched.
func LoadFile(path string) (*Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("HTML input must be a regular file")
	}
	return Parse(path, f)
}

func Parse(name string, r io.Reader) (*Document, error) {
	if r == nil {
		return nil, fmt.Errorf("HTML reader is nil")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("HTML must be UTF-8")
	}
	h, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	doc := &Document{Name: name, ID: fmt.Sprintf("%x", sha256.Sum256(data))}
	// The library has no font defaults. Seed our inspection subset through its
	// public extension API and reject declarations outside that known subset.
	// The former property-forking implementation could panic on missing defaults.
	extensions := []style.KeyValue{{Key: "font-family", Value: "serif"}, {Key: "font-size", Value: "medium"}, {Key: "font-weight", Value: "normal"}, {Key: "font-style", Value: "normal"}, {Key: "line-height", Value: "normal"}, {Key: "unicode-bidi", Value: "normal"}}
	defaults := style.InitializeDefaultPropertyValues(extensions)
	css := cssom.NewCSSOM(extensions)
	validate := func(declarations []*dcss.Declaration) error {
		seen := make(map[string]bool)
		for _, decl := range declarations {
			if decl.Important || strings.Contains(decl.Value, "!") || seen[decl.Property] {
				return fmt.Errorf("CSS !important and repeated declarations in one rule are unsupported by this cascade subset")
			}
			seen[decl.Property] = true
			properties, err := style.SplitCompoundProperty(decl.Property, style.Property(decl.Value))
			if err != nil {
				properties = []style.KeyValue{{Key: decl.Property, Value: style.Property(decl.Value)}}
			}
			for _, p := range properties {
				if _, ok := defaults.Property(p.Key); !ok {
					return fmt.Errorf("CSS property %q is not supported by the DOM inspection subset", p.Key)
				}
			}
		}
		return nil
	}
	// Extract style text ourselves: the library convenience helper dereferences
	// missing children on empty <style> elements and only scans head/body children.
	var sheets func(*html.Node) error
	sheets = func(n *html.Node) error {
		for _, a := range n.Attr {
			if a.Key == "style" {
				if strings.Contains(a.Val, "!") {
					return fmt.Errorf("inline CSS priority syntax is unsupported")
				}
				declarations, err := parser.ParseDeclarations(a.Val)
				if err != nil {
					return fmt.Errorf("inline CSS: %w", err)
				}
				if err = validate(declarations); err != nil {
					return err
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "style" {
			var text strings.Builder
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					text.WriteString(c.Data)
				}
			}
			sheet, err := parser.Parse(text.String())
			if err != nil {
				return fmt.Errorf("embedded CSS: %w", err)
			}
			for _, rule := range sheet.Rules {
				if rule.Kind != dcss.QualifiedRule {
					return fmt.Errorf("CSS at-rules are not supported by the DOM inspection subset")
				}
				if _, err := cascadia.Compile(rule.Prelude); err != nil {
					return fmt.Errorf("CSS selector: %w", err)
				}
				if err := validate(rule.Declarations); err != nil {
					return err
				}
			}
			if err = css.AddStylesForScope(nil, douceuradapter.Wrap(sheet), cssom.Author); err != nil {
				return err
			}
		}
		if n.Type == html.ElementNode && n.Data == "link" {
			doc.Warnings = append(doc.Warnings, "Linked resources are not loaded")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := sheets(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err = sheets(h); err != nil {
		return nil, err
	}
	root, err := css.Style(h)
	if err != nil {
		return nil, fmt.Errorf("style DOM: %w", err)
	}
	wrapper, err := dom.NodeFromTreeNode(root)
	if err != nil {
		return nil, err
	}
	// Use FirstChild/NextSibling, not Children(), whose filtered list can have nil entries.
	var visit func(*dom.W3CNode, int, int, string)
	visit = func(n *dom.W3CNode, parent, depth int, path string) {
		hn := n.HTMLNode()
		id := len(doc.Nodes)
		doc.Nodes = append(doc.Nodes, Node{ID: id, Parent: parent, Depth: depth, Path: path, Name: n.NodeName(), Text: n.NodeValue(), Type: hn.Type, Attributes: append([]html.Attribute(nil), hn.Attr...), styled: n.StyNode})
		if parent >= 0 {
			doc.Nodes[parent].Children = append(doc.Nodes[parent].Children, id)
		}
		i := 0
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			i++
			visit(c.(*dom.W3CNode), id, depth+1, fmt.Sprintf("%s/%s[%d]", path, c.NodeName(), i))
		}
	}
	visit(wrapper, -1, 0, "/#document")
	return doc, nil
}

// Property returns an inspected library value. It safely walks public property
// maps instead of css.GetCascadedProperty, which dereferences the root's parent.
// This is an inspection subset, not a browser computed-style implementation.
func (d *Document) Property(id int, key string) (string, error) {
	if id < 0 || id >= len(d.Nodes) {
		return "", fmt.Errorf("unknown DOM node %d", id)
	}
	sn := d.Nodes[id].styled
	if sn.HTMLNode().Type == html.TextNode && sn.Parent() != nil {
		sn = sn.Parent().Payload
	}
	origin := sn
	inherit := style.IsCascading(key) || strings.HasPrefix(key, "font-")
	for sn != nil {
		if value, ok := sn.Styles().Property(key); ok {
			if value != "inherit" && value != "unset" {
				return string(value), nil
			}
			inherit = true
		}
		if !inherit {
			break
		}
		parent := sn.Parent()
		if parent == nil {
			break
		}
		sn = parent.Payload
	}
	value := style.GetUserAgentDefaultProperty(origin.HTMLNode(), key)
	if value == style.NullStyle {
		return "", fmt.Errorf("no library value for %s", key)
	}
	return string(value), nil
}

var inlineTags = strings.Fields("a abbr b bdi cite code del dfn em i ins kbd mark q s samp small span strong sub sup time u var")
var rootTags = strings.Fields("p div li dt dd h1 h2 h3 h4 h5 h6 blockquote figcaption pre")

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (d *Document) checkVisibility(id int) error {
	n := d.Nodes[id]
	if n.Type != html.ElementNode {
		return nil
	}
	for _, a := range n.Attributes {
		if a.Key == "hidden" {
			return fmt.Errorf("%s: hidden content is not supported", n.Path)
		}
	}
	display, _ := d.Property(id, "display")
	visibility, _ := d.Property(id, "visibility")
	if display == "none" || visibility == "hidden" || visibility == "collapse" {
		return fmt.Errorf("%s: hidden content is not supported", n.Path)
	}
	return nil
}

// Preview returns a bounded, whitespace-preserving descendant-text preview.
func (d *Document) Preview(id, limit int) string {
	var runes []rune
	var walk func(int)
	walk = func(i int) {
		for _, r := range d.Nodes[i].Text {
			if len(runes) > limit {
				return
			}
			runes = append(runes, r)
		}
		for _, c := range d.Nodes[i].Children {
			if len(runes) > limit {
				return
			}
			walk(c)
		}
	}
	walk(id)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return string(runes)
}
