package server

// EditParams is the params_json shape for an 'edit' cell.
type EditParams struct {
	Margin string  `json:"margin"`
	Speed  float64 `json:"speed"`
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
