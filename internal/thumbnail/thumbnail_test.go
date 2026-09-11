package thumbnail

import (
	"strings"
	"testing"
)

func TestFitTitleShortTitleUsesLargestFont(t *testing.T) {
	lines, size := fitTitle("Hello World", 1136, maxLines)
	if len(lines) != 1 {
		t.Fatalf("lines = %v, want 1 line", lines)
	}
	if size != fontSizeSteps[0] {
		t.Fatalf("size = %v, want largest step %v", size, fontSizeSteps[0])
	}
}

func TestFitTitleLongTitleWrapsAndShrinks(t *testing.T) {
	title := "This Is A Considerably Longer Video Title That Should Wrap Across Multiple Lines"
	lines, size := fitTitle(title, 1136, maxLines)
	if len(lines) < 2 {
		t.Fatalf("expected multiple lines for a long title, got %v", lines)
	}
	if len(lines) > maxLines {
		t.Fatalf("lines = %d, want <= %d", len(lines), maxLines)
	}
	if size >= fontSizeSteps[0] {
		t.Fatalf("expected font size to shrink below the largest step for a long title, got %v", size)
	}
}

func TestFitTitleExtremelyLongTitleTruncatesWithEllipsis(t *testing.T) {
	title := strings.Repeat("Extremely Long Title Words Overflowing The Thumbnail Canvas Width Every Single Time ", 10)
	lines, _ := fitTitle(title, 1136, maxLines)
	if len(lines) != maxLines {
		t.Fatalf("lines = %d, want exactly %d (truncated)", len(lines), maxLines)
	}
	last := lines[maxLines-1]
	if !strings.HasSuffix(last, "…") {
		t.Fatalf("last line %q should end with an ellipsis", last)
	}
	if estWidth(last, fontSizeSteps[len(fontSizeSteps)-1]) > 1136 {
		t.Fatalf("truncated last line still estimates wider than the box: %q", last)
	}
}

func TestFitTitleEmptyTitleFallsBackToPlaceholder(t *testing.T) {
	lines, _ := fitTitle("   ", 1136, maxLines)
	if len(lines) != 1 || lines[0] != "Untitled" {
		t.Fatalf("lines = %v, want [\"Untitled\"]", lines)
	}
}

func TestWrapTitleSingleWordNeverSplits(t *testing.T) {
	lines := wrapTitle("Supercalifragilisticexpialidocious", 88, 100)
	if len(lines) != 1 {
		t.Fatalf("a single word must stay on one line even if it estimates wider than maxWidth, got %v", lines)
	}
}

func TestEscapeXMLHandlesSpecialCharacters(t *testing.T) {
	got := escapeXML(`Tom & Jerry's "Great" <Escape>`)
	want := "Tom &amp; Jerry&apos;s &quot;Great&quot; &lt;Escape&gt;"
	if got != want {
		t.Fatalf("escapeXML = %q, want %q", got, want)
	}
}

func TestLayoutLinesCentersVerticallyAndFlagsWideLines(t *testing.T) {
	lines := layoutLines([]string{"Short", strings.Repeat("Wide ", 40)}, 60, 1136)
	if len(lines) != 2 {
		t.Fatalf("expected 2 laid-out lines, got %d", len(lines))
	}
	if lines[0].Y >= lines[1].Y {
		t.Fatalf("expected line 2 to sit below line 1: y0=%v y1=%v", lines[0].Y, lines[1].Y)
	}
	if lines[0].UseTextLength {
		t.Fatal("short line should not need a textLength fit hint")
	}
	if !lines[1].UseTextLength {
		t.Fatal("very wide line should be flagged for an exact textLength fit")
	}
	// XML-escaping must have already happened.
	if lines[0].Text != "Short" {
		t.Fatalf("Text = %q", lines[0].Text)
	}
}

func TestRenderSVGProducesWellFormedLookingOutput(t *testing.T) {
	lines := layoutLines([]string{"My Title"}, 72, 1136)
	svg, err := renderSVG(svgData{
		BackgroundColor: "#0f172a",
		AccentColor:     "#22d3ee",
		TextColor:       "#ffffff",
		Lines:           lines,
		FontSize:        72,
		TextBoxWidth:    1136,
	})
	if err != nil {
		t.Fatalf("renderSVG: %v", err)
	}
	if !strings.Contains(svg, "<svg") || !strings.Contains(svg, "</svg>") {
		t.Fatalf("output doesn't look like an SVG document: %s", svg)
	}
	if !strings.Contains(svg, "My Title") {
		t.Fatalf("output missing title text: %s", svg)
	}
	if !strings.Contains(svg, "#0f172a") || !strings.Contains(svg, "#22d3ee") || !strings.Contains(svg, "#ffffff") {
		t.Fatalf("output missing configured colors: %s", svg)
	}
	if strings.Contains(svg, "<image") {
		t.Fatalf("no logo was configured but an <image> tag was emitted: %s", svg)
	}
}

func TestRenderSVGEmitsLogoWhenPresent(t *testing.T) {
	svg, err := renderSVG(svgData{
		BackgroundColor: "#000",
		AccentColor:     "#fff",
		TextColor:       "#fff",
		Lines:           layoutLines([]string{"Title"}, 72, 1136),
		FontSize:        72,
		TextBoxWidth:    1136,
		HasLogo:         true,
		LogoDataURI:     "data:image/png;base64,AAAA",
		LogoX:           900,
		LogoY:           500,
		LogoW:           200,
		LogoH:           100,
	})
	if err != nil {
		t.Fatalf("renderSVG: %v", err)
	}
	if !strings.Contains(svg, `xlink:href="data:image/png;base64,AAAA"`) {
		t.Fatalf("output missing logo data URI: %s", svg)
	}
}
