package server

import "math"

// ResolveProfile converts a ProfileParams template into concrete EditParams
// by resolving ScalePct against a source video's actual resolution.
// ScalePct <= 0 or >= 100, or an unknown (<= 0) source resolution, leaves
// Width/Height at 0, meaning "keep original" — the same convention EditParams
// itself uses.
func ResolveProfile(pp ProfileParams, sourceWidth, sourceHeight int) EditParams {
	ep := EditParams{
		Margin:      pp.Margin,
		Speed:       pp.Speed,
		BitrateKbps: pp.BitrateKbps,
		LockAspect:  pp.LockAspect,
		SkipDenoise: pp.SkipDenoise,
	}
	if pp.ScalePct > 0 && pp.ScalePct < 100 && sourceWidth > 0 && sourceHeight > 0 {
		ep.Width = int(math.Round(float64(sourceWidth) * pp.ScalePct / 100))
		ep.Height = int(math.Round(float64(sourceHeight) * pp.ScalePct / 100))
	}
	return ep
}

// DeriveProfileParams builds a ProfileParams template from an existing
// cell's EditParams, expressing its Width against sourceWidth as a
// percentage. If the source resolution is unknown (sourceWidth <= 0) or the
// cell has no resize set, ScalePct is left 0 rather than guessed.
func DeriveProfileParams(ep EditParams, sourceWidth, sourceHeight int) ProfileParams {
	pp := ProfileParams{
		Margin:      ep.Margin,
		Speed:       ep.Speed,
		BitrateKbps: ep.BitrateKbps,
		LockAspect:  ep.LockAspect,
		SkipDenoise: ep.SkipDenoise,
	}
	if ep.Width > 0 && sourceWidth > 0 {
		pp.ScalePct = math.Round(float64(ep.Width) / float64(sourceWidth) * 100)
	}
	return pp
}
