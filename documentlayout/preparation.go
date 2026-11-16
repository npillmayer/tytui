package documentlayout

import (
	"fmt"
	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/typography/paragraph"
)

// preparedGroup is private to one invocation and immutable after insertion.
// Source correspondence, paint and owner context share the same extraction for
// measurement and final layout. No CSS request views or measurement answers are
// retained. Operations on its paragraph (and the invocation) must be serialized.
type preparedGroup struct {
	input     InlineGroup
	paragraph *paragraph.Prepared
}

// Check structural identity even on cache hits: BoxIDs alone are not identities
// across normalized trees. Document and its tree remain stable for the invocation.
func (i *invocation) validateGroup(group *box.LayoutNode, owner box.BoxID) error {
	if group == nil {
		return invalid("missing inline group")
	}
	entry, ok := i.document.groups[group.BoxID]
	if !ok || group != entry.node || owner != entry.OwnerID {
		return invalid("unknown inline group or mismatched owner")
	}
	return nil
}

// prepareGroup is independent of final-callback tracking and accepted results.
// A failed attempt inserts nothing and can be retried; earlier complete entries
// survive. Final callback attempts retain their separate no-repeat contract.
func (i *invocation) prepareGroup(group *box.LayoutNode, owner box.BoxID) (*preparedGroup, error) {
	if err := i.ctx.Err(); err != nil {
		return nil, err
	}
	if err := i.validateGroup(group, owner); err != nil {
		return nil, err
	}
	if prepared := i.prepared[group.BoxID]; prepared != nil {
		return prepared, nil
	}
	i.trace(fmt.Sprintf("prepare group=%d owner=%d: itemize, shape and knot", group.BoxID, owner))
	g, err := i.document.Extract(i.ctx, group.BoxID)
	if err != nil {
		return nil, err
	}
	r := i.config.Resolver
	text, _, err := r.ItemizeInline(i.ctx, g.Text, g.Styles, g.Anchors)
	if err != nil {
		return nil, err
	}
	// Capture inherited owner typography once; final layout measures its strut.
	defaults, err := r.ResolveStyle(g.Paragraph.Style, r.Defaults())
	if err != nil {
		return nil, err
	}
	markup, err := g.BidiMarkup()
	if err != nil {
		return nil, err
	}
	// Build has checked authored markers against this fixed document policy.
	// Measurement and final layout reuse this preparation and its policy.
	p, err := paragraph.Prepare(i.ctx, paragraph.Input{Text: text, Direction: g.Paragraph.Style.Direction, Markup: markup, Atomics: g.Anchors},
		khipu.PrepareOptions{Hyphenation: khipu.NoHyphenation}, khipu.Config{Streaming: &khipu.StreamingConfig{
			Resolver: r, Defaults: defaults, NewShaper: i.config.NewShaper, BufferOptions: i.config.BufferOptions,
		}})
	if err != nil {
		return nil, err
	}
	if err := i.ctx.Err(); err != nil {
		return nil, err
	}
	prepared := &preparedGroup{input: *g, paragraph: p}
	i.prepared[group.BoxID] = prepared
	return prepared, nil
}
