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
