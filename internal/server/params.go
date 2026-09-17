package server

// EditParams is the params_json shape for an 'edit' cell.
type EditParams struct {
	Margin string  `json:"margin"`
	Speed  float64 `json:"speed"`

	// Width/Height/BitrateKbps request a resize/re-encode pass; 0 (the
	// default) keeps that dimension/the bitrate unchanged from the
	// source. LockAspect is UI-only state (whether changing Width/Height
	// should keep the other proportional) persisted here so it survives
	// reopening the cell; it has no effect on the pipeline itself.
	Width       int  `json:"width"`
	Height      int  `json:"height"`
	BitrateKbps int  `json:"bitrateKbps"`
	LockAspect  bool `json:"lockAspect"`

	// SkipDenoise disables the DeepFilterNet denoise stage, cutting
	// silence directly on the split audio instead. Named as a negative so
	// existing cells' params (saved before this field existed) default to
	// false, i.e. keep denoising on — the long-standing behavior.
	SkipDenoise bool `json:"skipDenoise"`
}

// ProfileParams is the params_json shape stored in a profiles row — a
// resolution-independent template for EditParams. ScalePct is a percentage
// of the target cell's source resolution (0 or >=100 means keep original);
// BitrateKbps stays absolute since bitrate doesn't scale predictably with
// resolution. LockAspect is carried through as-is (UI toggle state,
// restored verbatim on apply, not resolution-dependent).
type ProfileParams struct {
	Margin      string  `json:"margin"`
	Speed       float64 `json:"speed"`
	ScalePct    float64 `json:"scalePct"`
	BitrateKbps int     `json:"bitrateKbps"`
	LockAspect  bool    `json:"lockAspect"`
	SkipDenoise bool    `json:"skipDenoise"`
}

// TextParams is the params_json shape for a 'text' cell.
type TextParams struct {
	Markdown string `json:"markdown"`
}

// UploadParams is the params_json shape for an 'upload' cell.
type UploadParams struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Tags          []string `json:"tags"`
	Privacy       string   `json:"privacy"`
	Language      string   `json:"language"`
	ThumbnailMode string   `json:"thumbnailMode"` // "auto" | "custom" | "none"
	ThumbnailPath string   `json:"thumbnailPath"` // set when ThumbnailMode == "custom"
}
