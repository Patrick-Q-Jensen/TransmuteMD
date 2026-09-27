package analyze

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

const (
	minimumLineOverlapRatio = 0.5
	maximumLineCenterRatio  = 0.35
	wordGapRatio            = 0.2
	paragraphGapRatio       = 0.9
	paragraphIndentRatio    = 0.75
	shortLineGapRatio       = 2
	shortLineWidthRatio     = 0.15
)

var errNilLayout = errors.New("layout must not be nil")

// BasicAnalyzer produces plain paragraphs using deterministic single-column
// reading-order heuristics.
type BasicAnalyzer struct{}

var _ Analyzer = (*BasicAnalyzer)(nil)

// NewBasicAnalyzer creates a basic layout analyzer.
func NewBasicAnalyzer() *BasicAnalyzer {
	return &BasicAnalyzer{}
}

// Analyze orders physical text and groups nearby lines into paragraphs.
func (*BasicAnalyzer) Analyze(
	ctx context.Context,
	layout *document.Layout,
) (*document.Document, error) {
	if layout == nil {
		return nil, errNilLayout
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("analyze layout: %w", err)
	}
	if err := layout.Validate(); err != nil {
		return nil, fmt.Errorf("validate layout for analysis: %w", err)
	}

	result := &document.Document{}
	for _, page := range layout.Pages {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("analyze page %d: %w", page.Number, err)
		}

		paragraphs, err := analyzePage(ctx, page)
		if err != nil {
			return nil, fmt.Errorf("analyze page %d: %w", page.Number, err)
		}
		result.Blocks = append(result.Blocks, paragraphs...)
	}

	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("validate analyzed document: %w", err)
	}
	return result, nil
}

type orderedRun struct {
	run   document.TextRun
	text  string
	index int
}

type textLine struct {
	runs   []orderedRun
	top    float64
	bottom float64
	left   float64
	right  float64
	text   string
}

func analyzePage(ctx context.Context, page document.Page) ([]document.Block, error) {
	runs := make([]orderedRun, 0, len(page.TextRuns))
	for index, run := range page.TextRuns {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		text := normalizeRunText(run)
		if text == "" {
			continue
		}
		runs = append(runs, orderedRun{run: run, text: text, index: index})
	}

	slices.SortStableFunc(runs, compareRunsVertically)

	lines := make([]textLine, 0, len(runs))
	for _, run := range runs {
		if len(lines) == 0 || !belongsToLine(lines[len(lines)-1], run.run) {
			lines = append(lines, newTextLine(run))
			continue
		}
		lines[len(lines)-1].add(run)
	}

	for index := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := lines[index].finish(ctx); err != nil {
			return nil, err
		}
	}
	slices.SortStableFunc(lines, compareLines)

	return groupBlocks(ctx, lines)
}

func normalizeRunText(run document.TextRun) string {
	var result strings.Builder
	previousWasSpace := false
	for _, value := range run.Text {
		if unicode.IsSpace(value) {
			if !previousWasSpace {
				result.WriteByte(' ')
				previousWasSpace = true
			}
			continue
		}
		result.WriteRune(value)
		previousWasSpace = false
	}

	text := result.String()
	if text != " " || run.Bounds.Width() > 0 && run.Bounds.Height() > 0 {
		return text
	}
	return ""
}

func compareRunsVertically(left, right orderedRun) int {
	leftCenter := verticalCenter(left.run.Bounds)
	rightCenter := verticalCenter(right.run.Bounds)
	if order := compareFloat(leftCenter, rightCenter); order != 0 {
		return order
	}
	if order := compareFloat(left.run.Bounds.Left, right.run.Bounds.Left); order != 0 {
		return order
	}
	return left.index - right.index
}

func belongsToLine(line textLine, run document.TextRun) bool {
	lineHeight := line.bottom - line.top
	runHeight := run.Bounds.Height()
	overlap := math.Min(line.bottom, run.Bounds.Bottom) -
		math.Max(line.top, run.Bounds.Top)
	shorterHeight := math.Min(lineHeight, runHeight)
	if shorterHeight > 0 && overlap >= shorterHeight*minimumLineOverlapRatio {
		return true
	}

	centerDistance := math.Abs(
		(line.top+line.bottom)/2 - verticalCenter(run.Bounds),
	)
	return centerDistance <= math.Max(lineHeight, runHeight)*maximumLineCenterRatio
}

func newTextLine(run orderedRun) textLine {
	return textLine{
		runs:   []orderedRun{run},
		top:    run.run.Bounds.Top,
		bottom: run.run.Bounds.Bottom,
		left:   run.run.Bounds.Left,
		right:  run.run.Bounds.Right,
	}
}

func (line *textLine) add(run orderedRun) {
	line.runs = append(line.runs, run)
	line.top = math.Min(line.top, run.run.Bounds.Top)
	line.bottom = math.Max(line.bottom, run.run.Bounds.Bottom)
	line.left = math.Min(line.left, run.run.Bounds.Left)
	line.right = math.Max(line.right, run.run.Bounds.Right)
}

func (line *textLine) finish(ctx context.Context) error {
	slices.SortStableFunc(line.runs, func(left, right orderedRun) int {
		if order := compareFloat(left.run.Bounds.Left, right.run.Bounds.Left); order != 0 {
			return order
		}
		if order := compareFloat(left.run.Bounds.Top, right.run.Bounds.Top); order != 0 {
			return order
		}
		return left.index - right.index
	})

	var result strings.Builder
	var previous *orderedRun
	spacePending := false
	for index := range line.runs {
		if err := ctx.Err(); err != nil {
			return err
		}

		current := &line.runs[index]
		text := strings.TrimSpace(current.text)
		if text == "" {
			spacePending = result.Len() > 0
			previous = current
			continue
		}

		if result.Len() > 0 &&
			(spacePending ||
				strings.HasPrefix(current.text, " ") ||
				previous != nil && hasWordGap(previous.run, current.run)) {
			result.WriteByte(' ')
		}
		result.WriteString(text)
		spacePending = strings.HasSuffix(current.text, " ")
		previous = current
	}
	line.text = result.String()
	return nil
}

func hasWordGap(left, right document.TextRun) bool {
	gap := right.Bounds.Left - left.Bounds.Right
	if gap <= 0 {
		return false
	}

	leftScale := math.Max(left.Style.FontSize, left.Bounds.Height())
	rightScale := math.Max(right.Style.FontSize, right.Bounds.Height())
	scale := math.Max(leftScale, rightScale)
	if scale == 0 {
		return false
	}
	return gap > scale*wordGapRatio
}

func compareLines(left, right textLine) int {
	if order := compareFloat(left.top, right.top); order != 0 {
		return order
	}
	return compareFloat(left.left, right.left)
}

func groupBlocks(ctx context.Context, lines []textLine) ([]document.Block, error) {
	blocks := make([]document.Block, 0, len(lines))
	headingLevels := detectHeadingLevels(lines)
	pageLeft, pageRight := textMargins(lines, headingLevels)
	var paragraph []textLine

	flush := func() {
		if len(paragraph) == 0 {
			return
		}
		blocks = append(blocks, &document.Paragraph{Text: joinWrappedLines(paragraph)})
		paragraph = paragraph[:0]
	}

	for index, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if line.text == "" {
			continue
		}
		if headingLevels[index] != 0 {
			flush()
			blocks = append(blocks, &document.Heading{
				Level: headingLevels[index],
				Text:  line.text,
			})
			continue
		}
		if len(paragraph) > 0 &&
			startsNewParagraph(paragraph[len(paragraph)-1], line, pageLeft, pageRight) {
			flush()
		}
		paragraph = append(paragraph, line)
	}
	flush()

	return blocks, nil
}

func textMargins(lines []textLine, headingLevels []int) (float64, float64) {
	left := math.Inf(1)
	right := math.Inf(-1)
	for index, line := range lines {
		if line.text == "" || headingLevels[index] != 0 {
			continue
		}
		left = math.Min(left, line.left)
		right = math.Max(right, line.right)
	}
	return left, right
}

func detectHeadingLevels(lines []textLine) []int {
	levels := make([]int, len(lines))
	bodyScale := medianLineScale(lines)
	bodyWeight := medianLineWeight(lines)
	if bodyScale <= 0 {
		return levels
	}
	spacingThreshold := math.Max(bodyScale*0.35, medianLineGap(lines)*1.5)

	for index, line := range lines {
		if line.text == "" || !hasHeadingSpacing(lines, index, spacingThreshold) {
			continue
		}

		scaleRatio := line.scale() / bodyScale
		boldEvidence := line.weight() >= 600 &&
			line.weight() >= bodyWeight+200 &&
			hasStrongHeadingSpacing(lines, index, spacingThreshold)
		if scaleRatio < 1.15 && !boldEvidence {
			continue
		}

		switch {
		case scaleRatio >= 1.6:
			levels[index] = 1
		case scaleRatio >= 1.35:
			levels[index] = 2
		default:
			levels[index] = 3
		}
	}
	return levels
}

func medianLineScale(lines []textLine) float64 {
	values := make([]float64, 0, len(lines))
	for _, line := range lines {
		if line.text != "" && line.scale() > 0 {
			values = append(values, line.scale())
		}
	}
	if len(values) == 0 {
		return 0
	}
	slices.Sort(values)
	return values[(len(values)-1)/2]
}

func medianLineGap(lines []textLine) float64 {
	values := make([]float64, 0, len(lines)-1)
	for index := 1; index < len(lines); index++ {
		gap := lines[index].top - lines[index-1].bottom
		if gap >= 0 {
			values = append(values, gap)
		}
	}
	if len(values) == 0 {
		return 0
	}
	slices.Sort(values)
	return values[(len(values)-1)/2]
}

func medianLineWeight(lines []textLine) int {
	values := make([]int, 0, len(lines))
	for _, line := range lines {
		if line.text != "" {
			values = append(values, line.weight())
		}
	}
	if len(values) == 0 {
		return 0
	}
	slices.Sort(values)
	return values[(len(values)-1)/2]
}

func (line textLine) scale() float64 {
	scale := line.bottom - line.top
	for _, run := range line.runs {
		scale = math.Max(scale, run.run.Style.FontSize)
	}
	return scale
}

func (line textLine) weight() int {
	weight := 0
	for _, run := range line.runs {
		weight = max(weight, run.run.Style.FontWeight)
	}
	return weight
}

func hasHeadingSpacing(lines []textLine, index int, threshold float64) bool {
	return index == 0 ||
		index == len(lines)-1 ||
		lines[index].top-lines[index-1].bottom > threshold ||
		lines[index+1].top-lines[index].bottom > threshold
}

func hasStrongHeadingSpacing(lines []textLine, index int, threshold float64) bool {
	before := index == 0 || lines[index].top-lines[index-1].bottom > threshold
	after := index == len(lines)-1 || lines[index+1].top-lines[index].bottom > threshold
	return before && after
}

func startsNewParagraph(
	previous,
	current textLine,
	pageLeft,
	pageRight float64,
) bool {
	gap := current.top - previous.bottom
	lineHeight := math.Max(previous.bottom-previous.top, current.bottom-current.top)
	if lineHeight > 0 && gap > lineHeight*paragraphGapRatio {
		return true
	}

	indentThreshold := lineHeight * paragraphIndentRatio
	if current.left > pageLeft+indentThreshold &&
		previous.left <= pageLeft+indentThreshold/2 {
		return true
	}

	textWidth := pageRight - pageLeft
	shortLineGap := math.Max(
		lineHeight*shortLineGapRatio,
		textWidth*shortLineWidthRatio,
	)
	return endsSentence(previous.text) &&
		pageRight-previous.right > shortLineGap &&
		current.left <= pageLeft+indentThreshold
}

func joinWrappedLines(lines []textLine) string {
	var result strings.Builder
	for index, line := range lines {
		text := line.text
		if index+1 < len(lines) && endsWithSoftHyphen(text, lines[index+1].text) {
			text = strings.TrimSuffix(strings.TrimSuffix(text, "-"), "\u00ad")
		}
		result.WriteString(text)
		if index+1 < len(lines) && !endsWithSoftHyphen(line.text, lines[index+1].text) {
			result.WriteByte(' ')
		}
	}
	return result.String()
}

func endsSentence(text string) bool {
	text = strings.TrimRight(text, "\"')]}")
	if text == "" {
		return false
	}
	switch text[len(text)-1] {
	case '.', '!', '?', ':', ';':
		return true
	default:
		return false
	}
}

func endsWithSoftHyphen(text, next string) bool {
	if !strings.HasSuffix(text, "-") && !strings.HasSuffix(text, "\u00ad") {
		return false
	}
	first, _ := utf8.DecodeRuneInString(next)
	return unicode.IsLower(first)
}

func verticalCenter(bounds document.Rectangle) float64 {
	return (bounds.Top + bounds.Bottom) / 2
}

func compareFloat(left, right float64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
