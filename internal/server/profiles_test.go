package server

import "testing"

func TestResolveProfileScalePct(t *testing.T) {
	cases := []struct {
		name         string
		pp           ProfileParams
		srcW, srcH   int
		wantW, wantH int
	}{
		{"zero scale keeps original", ProfileParams{ScalePct: 0}, 1920, 1080, 0, 0},
		{"100 keeps original", ProfileParams{ScalePct: 100}, 1920, 1080, 0, 0},
		{"50 percent halves both dims", ProfileParams{ScalePct: 50}, 1920, 1080, 960, 540},
		{"unknown source resolution keeps original", ProfileParams{ScalePct: 50}, 0, 0, 0, 0},
		{"odd source width rounds", ProfileParams{ScalePct: 50}, 1917, 1081, 959, 541},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ep := ResolveProfile(c.pp, c.srcW, c.srcH)
			if ep.Width != c.wantW || ep.Height != c.wantH {
				t.Fatalf("ResolveProfile(%+v, %d, %d) = %d,%d, want %d,%d", c.pp, c.srcW, c.srcH, ep.Width, ep.Height, c.wantW, c.wantH)
			}
		})
	}
}

func TestResolveProfileCarriesOtherFields(t *testing.T) {
	pp := ProfileParams{Margin: "0.3s", Speed: 1.5, BitrateKbps: 4000, LockAspect: true, SkipDenoise: true}
	ep := ResolveProfile(pp, 0, 0)
	if ep.Margin != "0.3s" || ep.Speed != 1.5 || ep.BitrateKbps != 4000 || !ep.LockAspect || !ep.SkipDenoise {
		t.Fatalf("ResolveProfile did not carry through non-resize fields: %+v", ep)
	}
}

func TestDeriveProfileParamsRoundTrip(t *testing.T) {
	ep := EditParams{Margin: "0.2s", Speed: 1.25, Width: 960, Height: 540, BitrateKbps: 2000, LockAspect: true, SkipDenoise: true}
	pp := DeriveProfileParams(ep, 1920, 1080)
	if pp.ScalePct != 50 {
		t.Fatalf("ScalePct = %v, want 50", pp.ScalePct)
	}
	if pp.Margin != "0.2s" || pp.Speed != 1.25 || pp.BitrateKbps != 2000 || !pp.LockAspect || !pp.SkipDenoise {
		t.Fatalf("DeriveProfileParams did not carry through non-resize fields: %+v", pp)
	}

	// Round trip back through ResolveProfile against the same resolution.
	back := ResolveProfile(pp, 1920, 1080)
	if back.Width != ep.Width || back.Height != ep.Height {
		t.Fatalf("round trip = %d,%d, want %d,%d", back.Width, back.Height, ep.Width, ep.Height)
	}
}

func TestDeriveProfileParamsUnknownSourceLeavesScaleZero(t *testing.T) {
	ep := EditParams{Width: 960, Height: 540}
	pp := DeriveProfileParams(ep, 0, 0)
	if pp.ScalePct != 0 {
		t.Fatalf("ScalePct = %v, want 0 for unknown source resolution", pp.ScalePct)
	}
}
