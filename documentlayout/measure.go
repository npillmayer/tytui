package documentlayout

import (
	"context"
	"fmt"

	box "github.com/npillmayer/css-box-layout/layout"
	"github.com/npillmayer/khipu"
	"github.com/npillmayer/khipu/dimen"
)

// MeasureInline consumes invocation-scoped CSS views without retaining them.
// Core owns recursion, border sizing and shrink-to-fit selection. This callback
// measures just the requested group; it neither accepts final output nor marks
// a final callback attempt. Only completed horizontal preparation is cached.
func (i *invocation) MeasureInline(request box.InlineMeasureRequest) (box.IntrinsicWidths, error) {
	if err := i.ctx.Err(); err != nil {
		return box.IntrinsicWidths{}, err
	}
	if request.Group == nil || request.Edges == nil || request.Atomics == nil {
		return box.IntrinsicWidths{}, invalid("incomplete inline measurement request")
	}
	p, err := i.prepareGroup(request.Group, request.OwnerID)
	if err != nil {
		return box.IntrinsicWidths{}, err
	}
	atoms, err := atomicIntrinsics(i.ctx, &p.input, request)
	if err != nil {
		return box.IntrinsicWidths{}, err
	}
	widths, err := p.paragraph.MeasureIntrinsic(i.ctx, khipu.MeasureRequest{Atomics: atoms})
	if err != nil {
		return box.IntrinsicWidths{}, err
	}
	summary, err := intrinsicSummary(widths)
	if err != nil {
		return box.IntrinsicWidths{}, err
	}
	if err := i.ctx.Err(); err != nil {
		return box.IntrinsicWidths{}, err
	}
	return summary, nil
}

func atomicIntrinsics(ctx context.Context, g *InlineGroup, request box.InlineMeasureRequest) ([]khipu.AtomicIntrinsic, error) {
	atoms := make([]khipu.AtomicIntrinsic, 0, len(g.AtomicSources))
	for _, source := range g.AtomicSources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		atom, ok := request.Atomics.Lookup(source.BoxID)
		if !ok || atom.BoxID != source.BoxID {
			return nil, invalid("missing or mismatched intrinsic atomic box")
		}
		edges, ok := request.Edges.Lookup(source.BoxID)
		if !ok {
			return nil, invalid("missing atomic measurement edges")
		}
		// Check order before rounding: distinct invalid endpoints may quantize
		// to the same DU. cssToDU separately rejects nonfinite/negative bounds.
		if atom.BorderWidths.MinContent > atom.BorderWidths.MaxContent {
			return nil, invalid("reversed intrinsic atomic bounds")
		}
		var minimum, maximum, left, right dimen.DU
		for _, v := range []struct {
			px     float32
			signed bool
			dest   *dimen.DU
		}{
			{atom.BorderWidths.MinContent, false, &minimum}, {atom.BorderWidths.MaxContent, false, &maximum},
			{edges.Margin.Left, true, &left}, {edges.Margin.Right, true, &right},
		} {
			value, err := cssToDU(v.px, v.signed)
			if err != nil {
				return nil, fmt.Errorf("intrinsic atomic box %d: %w", source.BoxID, err)
			}
			*v.dest = value
		}
		// Each operand is finite DU, so these three-term int64 sums cannot
		// overflow. Check completed signed advances before narrowing; do not
		// clamp negative margins or reject a wide intermediate prefix.
		lo := int64(minimum) + int64(left) + int64(right)
		hi := int64(maximum) + int64(left) + int64(right)
		if lo <= -int64(dimen.Fil) || hi >= int64(dimen.Fil) || lo > hi {
			return nil, invalid("atomic intrinsic advance outside finite DU domain")
		}
		// Border bounds already include padding/borders. Owner edges and
		// vertical dimensions never contribute to this group's intrinsic width.
		atoms = append(atoms, khipu.AtomicIntrinsic{ID: source.ID, MinAdvance: dimen.DU(lo), MaxAdvance: dimen.DU(hi)})
	}
	return atoms, nil
}

func intrinsicSummary(w khipu.IntrinsicWidths) (box.IntrinsicWidths, error) {
	if w.MinContent < 0 || w.MaxContent < w.MinContent {
		return box.IntrinsicWidths{}, invalid("invalid paragraph intrinsic bounds")
	}
	minimum, err := pointsToCSS(w.MinContent.Points(), false)
	if err != nil {
		return box.IntrinsicWidths{}, err
	}
	maximum, err := pointsToCSS(w.MaxContent.Points(), false)
	if err != nil {
		return box.IntrinsicWidths{}, err
	}
	if minimum > maximum {
		return box.IntrinsicWidths{}, invalid("reversed CSS intrinsic summary")
	}
	return box.IntrinsicWidths{MinContent: minimum, MaxContent: maximum}, nil
}
