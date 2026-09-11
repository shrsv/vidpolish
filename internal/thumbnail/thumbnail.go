// Package thumbnail generates a 1280x720 YouTube thumbnail (brand
// background + optional logo + title text) as an SVG, then rasterizes it
// to PNG with resvg. The intermediate SVG is written alongside the PNG so
// it stays human-inspectable and hand-editable.
package thumbnail

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"vidpolish/internal/binmgr"
)

const (
	canvasWidth  = 1280
	canvasHeight = 720
	marginX      = 72
	maxLines     = 3

	logoMaxHeight = 120.0
	logoMaxWidth  = 320.0
	logoPadding   = 48.0
)

// fontSizeSteps are tried largest-first until the title fits within
// maxLines at the computed text box width.
var fontSizeSteps = []float64{88, 76, 66, 56, 48, 40, 34}

// approxCharWidthFactor estimates a glyph's average advance width as a
// fraction of font size for Inter Bold. This is a heuristic, not exact
// text measurement (resvg gives no such feedback over the CLI); each
// candidate line is also given an SVG textLength hint so the actually
// rendered width is exact even when this estimate is off.
const approxCharWidthFactor = 0.56

// Options configures a thumbnail generation.
type Options struct {
	Title           string
	LogoPath        string // optional; "" = no logo. PNG/JPG, or SVG (rasterized via resvg first)
	BackgroundColor string
	AccentColor     string
	TextColor       string
	OutPath         string // final PNG path; a sibling .svg is written alongside it
}

// Generate renders opts into a PNG thumbnail and returns its path.
func Generate(opts Options) (string, error) {
	resvgPath, err := binmgr.Resolve(binmgr.Resvg)
	if err != nil {
		return "", err
	}
	regularFont, boldFont, err := binmgr.ResolveFont()
	if err != nil {
		return "", err
	}

	logoDataURI, logoW, logoH, err := prepareLogo(resvgPath, opts.LogoPath)
	if err != nil {
		return "", fmt.Errorf("preparing logo: %w", err)
	}

	textBoxWidth := float64(canvasWidth - 2*marginX)
	lines, fontSize := fitTitle(opts.Title, textBoxWidth, maxLines)

	svg, err := renderSVG(svgData{
		BackgroundColor: opts.BackgroundColor,
		AccentColor:     opts.AccentColor,
		TextColor:       opts.TextColor,
		Lines:           layoutLines(lines, fontSize, textBoxWidth),
		FontSize:        fontSize,
		TextBoxWidth:    textBoxWidth,
		HasLogo:         logoDataURI != "",
		LogoDataURI:     logoDataURI,
		LogoX:           canvasWidth - logoPadding - logoW,
		LogoY:           canvasHeight - logoPadding - logoH,
		LogoW:           logoW,
		LogoH:           logoH,
	})
	if err != nil {
		return "", fmt.Errorf("rendering SVG template: %w", err)
	}

	svgPath := strings.TrimSuffix(opts.OutPath, filepath.Ext(opts.OutPath)) + ".svg"
	if err := os.MkdirAll(filepath.Dir(opts.OutPath), 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}
	if err := os.WriteFile(svgPath, []byte(svg), 0o644); err != nil {
		return "", fmt.Errorf("writing thumbnail SVG: %w", err)
	}

	cmd := exec.Command(resvgPath,
		"--use-font-file", regularFont,
		"--use-font-file", boldFont,
		"--skip-system-fonts",
		"-w", fmt.Sprint(canvasWidth),
		"-h", fmt.Sprint(canvasHeight),
		svgPath, opts.OutPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("resvg failed: %w\n%s", err, out)
	}

	return opts.OutPath, nil
}

// prepareLogo returns a data: URI for the logo plus its display
// width/height (aspect-ratio preserved, capped to logoMaxWidth/Height).
// SVG logos are rasterized once via resvg and cached by content hash. An
// empty LogoPath returns ("", 0, 0, nil).
func prepareLogo(resvgPath, logoPath string) (dataURI string, w, h float64, err error) {
	if logoPath == "" {
		return "", 0, 0, nil
	}

	pngPath := logoPath
	if strings.EqualFold(filepath.Ext(logoPath), ".svg") {
		pngPath, err = rasterizeSVGLogo(resvgPath, logoPath)
		if err != nil {
			return "", 0, 0, err
		}
	}

	data, err := os.ReadFile(pngPath)
	if err != nil {
		return "", 0, 0, fmt.Errorf("reading logo: %w", err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, fmt.Errorf("decoding logo image: %w", err)
	}

	scale := logoMaxHeight / float64(cfg.Height)
	if float64(cfg.Width)*scale > logoMaxWidth {
		scale = logoMaxWidth / float64(cfg.Width)
	}
	w = float64(cfg.Width) * scale
	h = float64(cfg.Height) * scale

	mime := "image/png"
	if format == "jpeg" {
		mime = "image/jpeg"
	}
	dataURI = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	return dataURI, w, h, nil
}

// rasterizeSVGLogo renders an SVG logo to a cached PNG (by content hash)
// at a fixed height, returning the cached PNG's path.
func rasterizeSVGLogo(resvgPath, svgPath string) (string, error) {
	data, err := os.ReadFile(svgPath)
	if err != nil {
		return "", fmt.Errorf("reading logo SVG: %w", err)
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])[:16]

	userCache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving user cache dir: %w", err)
	}
	dir := filepath.Join(userCache, "vidpolish", "thumbnail-logos")
	pngPath := filepath.Join(dir, hash+".png")

	if _, err := os.Stat(pngPath); err == nil {
		return pngPath, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating logo cache dir: %w", err)
	}

	// Render at a fixed height with resvg computing a proportional width
	// (-h alone preserves aspect ratio).
	cmd := exec.Command(resvgPath, "--skip-system-fonts", "-h", fmt.Sprint(int(logoMaxHeight)*2), svgPath, pngPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("rasterizing logo with resvg: %w\n%s", err, out)
	}
	return pngPath, nil
}

// wrapTitle greedily word-wraps title into lines that estimate to at most
// maxWidth pixels at fontSize.
func wrapTitle(title string, fontSize, maxWidth float64) []string {
	words := strings.Fields(title)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		candidate := cur + " " + w
		if estWidth(candidate, fontSize) <= maxWidth {
			cur = candidate
		} else {
			lines = append(lines, cur)
			cur = w
		}
	}
	lines = append(lines, cur)
	return lines
}

func estWidth(s string, fontSize float64) float64 {
	return float64(len([]rune(s))) * fontSize * approxCharWidthFactor
}

// fitTitle picks the largest font size (from fontSizeSteps) whose greedy
// word-wrap fits within maxLines at maxWidth, truncating the last line
// with an ellipsis at the smallest size if it still doesn't fit.
func fitTitle(title string, maxWidth float64, maxLines int) (lines []string, fontSize float64) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Untitled"
	}

	for _, size := range fontSizeSteps {
		wrapped := wrapTitle(title, size, maxWidth)
		if len(wrapped) <= maxLines {
			return wrapped, size
		}
	}

	size := fontSizeSteps[len(fontSizeSteps)-1]
	wrapped := wrapTitle(title, size, maxWidth)
	wrapped = wrapped[:maxLines]
	last := wrapped[maxLines-1]
	for estWidth(last+"…", size) > maxWidth && len(last) > 0 {
		r := []rune(last)
		last = strings.TrimSpace(string(r[:len(r)-1]))
	}
	wrapped[maxLines-1] = last + "…"
	return wrapped, size
}

type svgLine struct {
	Text          string
	Y             float64
	UseTextLength bool
}

// layoutLines computes vertical positions for a centered text block, and
// flags lines whose estimated width is close to/over maxWidth so the
// template applies an exact-fit textLength (making the actually rendered
// width correct regardless of the estimate's error margin).
func layoutLines(lines []string, fontSize, maxWidth float64) []svgLine {
	lineHeight := fontSize * 1.2
	totalHeight := float64(len(lines)) * lineHeight
	startY := float64(canvasHeight)/2 - totalHeight/2 + fontSize*0.35

	out := make([]svgLine, len(lines))
	for i, l := range lines {
		out[i] = svgLine{
			Text:          escapeXML(l),
			Y:             startY + float64(i)*lineHeight,
			UseTextLength: estWidth(l, fontSize) >= maxWidth*0.85,
		}
	}
	return out
}

func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

type svgData struct {
	BackgroundColor string
	AccentColor     string
	TextColor       string
	Lines           []svgLine
	FontSize        float64
	TextBoxWidth    float64
	HasLogo         bool
	LogoDataURI     string
	LogoX, LogoY    float64
	LogoW, LogoH    float64
}

var svgTemplate = template.Must(template.New("thumbnail").Parse(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="1280" height="720" viewBox="0 0 1280 720">
  <defs>
    <linearGradient id="tint" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="{{.AccentColor}}" stop-opacity="0"/>
      <stop offset="100%" stop-color="{{.AccentColor}}" stop-opacity="0.18"/>
    </linearGradient>
  </defs>
  <rect width="1280" height="720" fill="{{.BackgroundColor}}"/>
  <rect width="1280" height="720" fill="url(#tint)"/>
  <rect x="0" y="0" width="10" height="720" fill="{{.AccentColor}}"/>
{{- range .Lines}}
  <text x="72" y="{{.Y}}" font-family="Inter" font-weight="700" font-size="{{$.FontSize}}" fill="{{$.TextColor}}"{{if .UseTextLength}} textLength="{{$.TextBoxWidth}}" lengthAdjust="spacingAndGlyphs"{{end}}>{{.Text}}</text>
{{- end}}
{{- if .HasLogo}}
  <image x="{{.LogoX}}" y="{{.LogoY}}" width="{{.LogoW}}" height="{{.LogoH}}" xlink:href="{{.LogoDataURI}}"/>
{{- end}}
</svg>
`))

func renderSVG(data svgData) (string, error) {
	var buf strings.Builder
	if err := svgTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
