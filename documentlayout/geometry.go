package documentlayout

import (
	"math"

	"github.com/npillmayer/khipu/dimen"
)

const duPerCSSPixel = 49152.0 // 96 CSS px = 72 big points; dimen.PX is not CSS px.

// Convert each primitive once, never a pre-summed margin box. Check the rounded
// value before the integer cast, including the reserved fill-sentinel domain.
func cssToDU(px float32, signed bool) (dimen.DU, error) {
	v := float64(px)
	if math.IsNaN(v) || math.IsInf(v, 0) || (!signed && v < 0) {
		return 0, invalid("non-finite or negative CSS dimension")
	}
	v = math.Round(v * duPerCSSPixel)
	if math.Abs(v) >= float64(dimen.Fil) {
		return 0, invalid("CSS dimension outside finite DU domain")
	}
	return dimen.DU(v), nil
}

// Paragraph geometry remains float64 big points in retained results. Narrow only
// CSS summaries, rejecting even a rounding-induced excursion into the sentinel
// domain. Per primitive error is <= 0.5 DU, plus this float32 boundary rounding.
func pointsToCSS(points float64, signed bool) (float32, error) {
	if math.IsNaN(points) || math.IsInf(points, 0) || (!signed && points < 0) || math.Abs(points)*float64(dimen.BP) >= float64(dimen.Fil) {
		return 0, invalid("paragraph geometry outside finite DU domain")
	}
	px := float32(points * (4.0 / 3.0))
	if math.Abs(float64(px))*duPerCSSPixel >= float64(dimen.Fil) {
		return 0, invalid("CSS summary rounds outside finite DU domain")
	}
	return px, nil
}
