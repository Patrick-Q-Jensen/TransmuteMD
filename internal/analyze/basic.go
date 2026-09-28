package analyze

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
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
	listMarkerAlignRatio    = 0.5
	maximumListMarkerDigits = 9
	columnGutterScaleRatio  = 1.5
	columnGutterWidthRatio  = 0.03
	minimumColumnLines      = 2
	pageFurnitureBandRatio  = 0.12
	minimumFurniturePages   = 3
	pdfDiscretionaryBreak   = '\u0002'
	maximumTrackingGapRatio = 0.35
	trackingGapMultiplier   = 1.5
	minimumTrackingGaps     = 2
)

var errNilLayout = errors.New("layout must not be nil")

// BasicAnalyzer produces semantic blocks using deterministic single-column
// reading-order heuristics.
type BasicAnalyzer struct{}

var _ Analyzer = (*BasicAnalyzer)(nil)

// NewBasicAnalyzer creates a basic layout analyzer.
func NewBasicAnalyzer() *BasicAnalyzer {
	return &BasicAnalyzer{}
}

// Analyze orders physical text and groups lines into semantic blocks.
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

	pages := make([]analyzedPage, 0, len(layout.Pages))
	for _, page := range layout.Pages {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("analyze page %d: %w", page.Number, err)
		}

		lines, diagnostics, err := analyzePageLines(ctx, page)
		if err != nil {
			return nil, fmt.Errorf("analyze page %d: %w", page.Number, err)
		}
		pages = append(pages, analyzedPage{
			page:        page,
			lines:       lines,
			diagnostics: diagnostics,
		})
	}
	if err := suppressRepeatedPageFurniture(ctx, pages); err != nil {
		return nil, fmt.Errorf("detect repeated page headers and footers: %w", err)
	}

	result := &document.Document{}
	result.Diagnostics = slices.Clone(layout.Diagnostics)
	for _, page := range pages {
		result.Diagnostics = append(result.Diagnostics, page.diagnostics...)
		result.Diagnostics = append(
			result.Diagnostics,
			structureDiagnostics(page.page.Number, page.lines)...,
		)
		blocks, err := groupBlocks(ctx, page.lines)
		if err != nil {
			return nil, fmt.Errorf("analyze page %d: %w", page.page.Number, err)
		}
		result.Blocks = append(result.Blocks, blocks...)
	}

	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("validate analyzed document: %w", err)
	}
	return result, nil
}

type orderedRun struct {
	run             document.TextRun
	text            string
	index           int
	linkDestination string
}

type textLine struct {
	runs        []orderedRun
	top         float64
	bottom      float64
	left        float64
	right       float64
	text        string
	links       []document.TextLink
	breakBefore bool
}

type analyzedPage struct {
	page        document.Page
	lines       []textLine
	diagnostics []document.Diagnostic
}

func analyzePageLines(
	ctx context.Context,
	page document.Page,
) ([]textLine, []document.Diagnostic, error) {
	runs := make([]orderedRun, 0, len(page.TextRuns))
	ambiguousLink := false
	for index, run := range page.TextRuns {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		text := normalizeRunText(run)
		if text == "" {
			continue
		}
		destination, ambiguous := linkDestination(run.Bounds, page.Links)
		ambiguousLink = ambiguousLink || ambiguous
		runs = append(runs, orderedRun{
			run:             run,
			text:            text,
			index:           index,
			linkDestination: destination,
		})
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
	lines = splitCenterGutterLines(lines, page.Width)

	for index := range lines {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if err := lines[index].finish(ctx); err != nil {
			return nil, nil, err
		}
	}
	slices.SortStableFunc(lines, compareLines)
	lines = orderColumns(lines)

	var diagnostics []document.Diagnostic
	if ambiguousLink {
		diagnostics = append(diagnostics, document.Diagnostic{
			Code:    document.DiagnosticAmbiguousLink,
			Page:    page.Number,
			Message: "overlapping link annotations were preserved as plain text",
		})
	}
	return lines, diagnostics, nil
}

func normalizeRunText(run document.TextRun) string {
	var result strings.Builder
	previousWasSpace := false
	for _, value := range run.Text {
		if value == pdfDiscretionaryBreak {
			result.WriteByte('-')
			previousWasSpace = false
			continue
		}
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

func linkDestination(
	bounds document.Rectangle,
	links []document.LinkAnnotation,
) (string, bool) {
	centerX := (bounds.Left + bounds.Right) / 2
	centerY := (bounds.Top + bounds.Bottom) / 2
	destination := ""
	for _, link := range links {
		if centerX < link.Bounds.Left || centerX > link.Bounds.Right ||
			centerY < link.Bounds.Top || centerY > link.Bounds.Bottom {
			continue
		}
		if destination != "" && destination != link.Destination {
			return "", true
		}
		destination = link.Destination
	}
	return destination, false
}

func structureDiagnostics(page int, lines []textLine) []document.Diagnostic {
	var diagnostics []document.Diagnostic
	if hasTableLikeRegion(lines) {
		diagnostics = append(diagnostics, document.Diagnostic{
			Code:    document.DiagnosticTableLikeText,
			Page:    page,
			Message: "table-like layout was preserved as plain text",
		})
	}
	if hasCodeLikeRegion(lines) {
		diagnostics = append(diagnostics, document.Diagnostic{
			Code:    document.DiagnosticCodeLikeText,
			Page:    page,
			Message: "code-like layout was preserved as plain text",
		})
	}
	return diagnostics
}

func hasTableLikeRegion(lines []textLine) bool {
	consecutive := 0
	for _, line := range lines {
		if lineLargeGapCount(line) >= 2 {
			consecutive++
			if consecutive >= 3 {
				return true
			}
		} else {
			consecutive = 0
		}
	}
	return false
}

func lineLargeGapCount(line textLine) int {
	count := 0
	threshold := line.scale() * 2
	for index := 1; index < len(line.runs); index++ {
		if line.runs[index].run.Bounds.Left-
			line.runs[index-1].run.Bounds.Right > threshold {
			count++
		}
	}
	return count
}

func hasCodeLikeRegion(lines []textLine) bool {
	consecutive := 0
	previousLeft := 0.0
	for _, line := range lines {
		if !lineUsesMonospacedFont(line) {
			consecutive = 0
			continue
		}
		if consecutive > 0 &&
			math.Abs(line.left-previousLeft) > line.scale()*paragraphIndentRatio {
			consecutive = 0
		}
		consecutive++
		if consecutive >= 2 {
			return true
		}
		previousLeft = line.left
	}
	return false
}

func lineUsesMonospacedFont(line textLine) bool {
	hasText := false
	for _, run := range line.runs {
		if strings.TrimSpace(run.text) == "" {
			continue
		}
		name := strings.ToLower(run.run.Style.FontName)
		if !strings.Contains(name, "mono") &&
			!strings.Contains(name, "courier") &&
			!strings.Contains(name, "consolas") {
			return false
		}
		hasText = true
	}
	return hasText
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

func splitCenterGutterLines(lines []textLine, pageWidth float64) []textLine {
	pageCenter := pageWidth / 2
	splits := make([]int, len(lines))
	candidates := 0
	for lineIndex := range lines {
		line := &lines[lineIndex]
		slices.SortStableFunc(line.runs, func(left, right orderedRun) int {
			return compareFloat(left.run.Bounds.Left, right.run.Bounds.Left)
		})

		splits[lineIndex] = -1
		for index := 1; index < len(line.runs); index++ {
			left := line.runs[index-1].run
			right := line.runs[index].run
			gap := right.Bounds.Left - left.Bounds.Right
			scale := math.Max(
				math.Max(left.Style.FontSize, left.Bounds.Height()),
				math.Max(right.Style.FontSize, right.Bounds.Height()),
			)
			threshold := math.Max(
				scale*columnGutterScaleRatio,
				pageWidth*columnGutterWidthRatio,
			)
			if left.Bounds.Right <= pageCenter &&
				right.Bounds.Left >= pageCenter &&
				gap > threshold {
				splits[lineIndex] = index
				candidates++
				break
			}
		}
	}
	if candidates < minimumColumnLines {
		return lines
	}

	result := make([]textLine, 0, len(lines)+candidates)
	for lineIndex, line := range lines {
		split := splits[lineIndex]
		if split < 0 {
			result = append(result, line)
			continue
		}

		left := newTextLine(line.runs[0])
		for _, run := range line.runs[1:split] {
			left.add(run)
		}
		right := newTextLine(line.runs[split])
		for _, run := range line.runs[split+1:] {
			right.add(run)
		}
		result = append(result, left, right)
	}
	return result
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
	trackingGapThreshold := line.trackingGapThreshold()
	activeLink := ""
	activeStart := 0
	closeLink := func() {
		if activeLink == "" {
			return
		}
		line.links = append(line.links, document.TextLink{
			Start:       activeStart,
			End:         result.Len(),
			Destination: activeLink,
		})
		activeLink = ""
	}
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

		addSpace := result.Len() > 0 &&
			(spacePending ||
				strings.HasPrefix(current.text, " ") ||
				previous != nil && hasWordGap(
					previous.run,
					current.run,
					trackingGapThreshold,
				))
		if current.linkDestination != activeLink {
			closeLink()
		}
		if addSpace {
			result.WriteByte(' ')
		}
		if current.linkDestination != "" && current.linkDestination != activeLink {
			activeLink = current.linkDestination
			activeStart = result.Len()
		}
		result.WriteString(text)
		spacePending = strings.HasSuffix(current.text, " ")
		previous = current
	}
	closeLink()
	line.text = result.String()
	return nil
}

func (line textLine) trackingGapThreshold() float64 {
	gaps := make([]float64, 0, len(line.runs)-1)
	for index := 1; index < len(line.runs); index++ {
		left := line.runs[index-1]
		right := line.runs[index]
		if !isSingleWordRune(left.text) ||
			!isSingleWordRune(right.text) ||
			!sameTrackingStyle(left.run.Style, right.run.Style) {
			continue
		}

		gap := right.run.Bounds.Left - left.run.Bounds.Right
		scale := math.Max(
			math.Max(left.run.Style.FontSize, left.run.Bounds.Height()),
			math.Max(right.run.Style.FontSize, right.run.Bounds.Height()),
		)
		if gap > 0 && scale > 0 && gap <= scale*maximumTrackingGapRatio {
			gaps = append(gaps, gap)
		}
	}
	if len(gaps) < minimumTrackingGaps {
		return 0
	}
	slices.Sort(gaps)
	return gaps[(len(gaps)-1)/2] * trackingGapMultiplier
}

func isSingleWordRune(text string) bool {
	value, size := utf8.DecodeRuneInString(text)
	return size == len(text) && (unicode.IsLetter(value) || unicode.IsDigit(value))
}

func sameTrackingStyle(left, right document.TextStyle) bool {
	return left.FontName == right.FontName &&
		left.FontSize == right.FontSize &&
		left.FontWeight == right.FontWeight &&
		left.Italic == right.Italic
}

func hasWordGap(
	left,
	right document.TextRun,
	trackingGapThreshold float64,
) bool {
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
	return gap > math.Max(scale*wordGapRatio, trackingGapThreshold)
}

func compareLines(left, right textLine) int {
	if order := compareFloat(left.top, right.top); order != 0 {
		return order
	}
	return compareFloat(left.left, right.left)
}

func orderColumns(lines []textLine) []textLine {
	if len(lines) < minimumColumnLines*2 {
		return lines
	}

	contentLeft := math.Inf(1)
	contentRight := math.Inf(-1)
	for _, line := range lines {
		if line.text == "" {
			continue
		}
		contentLeft = math.Min(contentLeft, line.left)
		contentRight = math.Max(contentRight, line.right)
	}
	if math.IsInf(contentLeft, 1) || contentRight <= contentLeft {
		return lines
	}

	center := (contentLeft + contentRight) / 2
	gutter := math.Max(
		medianLineScale(lines)*columnGutterScaleRatio,
		(contentRight-contentLeft)*columnGutterWidthRatio,
	)
	leftLimit := center - gutter/2
	rightLimit := center + gutter/2

	result := make([]textLine, 0, len(lines))
	for start := 0; start < len(lines); {
		if spansColumnGutter(lines[start], leftLimit, rightLimit) {
			result = append(result, lines[start])
			start++
			continue
		}

		end := start
		for end < len(lines) &&
			!spansColumnGutter(lines[end], leftLimit, rightLimit) {
			end++
		}
		region, ok := orderColumnRegion(lines[start:end], leftLimit, rightLimit)
		if !ok {
			result = append(result, lines[start:end]...)
		} else {
			if len(result) > 0 {
				region[0].breakBefore = true
			}
			result = append(result, region...)
		}
		start = end
	}
	return result
}

func spansColumnGutter(line textLine, leftLimit, rightLimit float64) bool {
	return line.left < rightLimit && line.right > leftLimit
}

func orderColumnRegion(
	lines []textLine,
	leftLimit,
	rightLimit float64,
) ([]textLine, bool) {
	left := make([]textLine, 0, len(lines))
	right := make([]textLine, 0, len(lines))
	for _, line := range lines {
		switch {
		case line.right <= leftLimit:
			left = append(left, line)
		case line.left >= rightLimit:
			right = append(right, line)
		default:
			return nil, false
		}
	}
	if len(left) < minimumColumnLines || len(right) < minimumColumnLines {
		return nil, false
	}
	if !verticalRangesOverlap(left, right) {
		return nil, false
	}

	ordered := make([]textLine, 0, len(lines))
	ordered = append(ordered, left...)
	right[0].breakBefore = true
	ordered = append(ordered, right...)
	return ordered, true
}

func verticalRangesOverlap(left, right []textLine) bool {
	leftTop, leftBottom := lineRange(left)
	rightTop, rightBottom := lineRange(right)
	return math.Min(leftBottom, rightBottom) > math.Max(leftTop, rightTop)
}

func lineRange(lines []textLine) (float64, float64) {
	top := math.Inf(1)
	bottom := math.Inf(-1)
	for _, line := range lines {
		top = math.Min(top, line.top)
		bottom = math.Max(bottom, line.bottom)
	}
	return top, bottom
}

func suppressRepeatedPageFurniture(
	ctx context.Context,
	pages []analyzedPage,
) error {
	if len(pages) < minimumFurniturePages {
		return nil
	}

	counts := make(map[string]int)
	for _, page := range pages {
		seen := make(map[string]struct{})
		for _, line := range page.lines {
			if err := ctx.Err(); err != nil {
				return err
			}
			signature, ok := pageFurnitureSignature(page.page, line)
			if !ok {
				continue
			}
			seen[signature] = struct{}{}
		}
		for signature := range seen {
			counts[signature]++
		}
	}

	repeated := make(map[string]struct{})
	for signature, count := range counts {
		if count >= minimumFurniturePages && count*3 >= len(pages)*2 {
			repeated[signature] = struct{}{}
		}
	}
	if len(repeated) == 0 {
		return nil
	}

	for pageIndex := range pages {
		filtered := pages[pageIndex].lines[:0]
		for _, line := range pages[pageIndex].lines {
			signature, ok := pageFurnitureSignature(pages[pageIndex].page, line)
			if _, suppress := repeated[signature]; ok && suppress {
				continue
			}
			filtered = append(filtered, line)
		}
		pages[pageIndex].lines = filtered
	}
	return nil
}

func pageFurnitureSignature(page document.Page, line textLine) (string, bool) {
	zone := ""
	switch {
	case line.bottom <= page.Height*pageFurnitureBandRatio:
		zone = "header"
	case line.top >= page.Height*(1-pageFurnitureBandRatio):
		zone = "footer"
	default:
		return "", false
	}

	center := (line.left + line.right) / 2
	alignment := "center"
	if center < page.Width/3 {
		alignment = "left"
	} else if center > page.Width*2/3 {
		alignment = "right"
	}
	text := normalizeFurnitureText(line.text)
	if text == "" {
		return "", false
	}
	return zone + "\x00" + alignment + "\x00" + text, true
}

func normalizeFurnitureText(text string) string {
	var result strings.Builder
	inDigits := false
	for _, value := range strings.ToLower(strings.TrimSpace(text)) {
		if unicode.IsDigit(value) {
			if !inDigits {
				result.WriteByte('#')
			}
			inDigits = true
			continue
		}
		inDigits = false
		if unicode.IsSpace(value) {
			if result.Len() > 0 {
				result.WriteByte(' ')
			}
			continue
		}
		result.WriteRune(value)
	}
	return strings.TrimSpace(result.String())
}

func groupBlocks(ctx context.Context, lines []textLine) ([]document.Block, error) {
	blocks := make([]document.Block, 0, len(lines))
	headingLevels := detectHeadingLevels(lines)
	pageLeft, pageRight := textMargins(lines, headingLevels)
	var paragraph []textLine
	var list *pendingList

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		text, links := joinWrappedContent(paragraph)
		blocks = append(blocks, &document.Paragraph{Text: text, Links: links})
		paragraph = paragraph[:0]
	}
	flushList := func() {
		if list == nil {
			return
		}
		blocks = append(blocks, list.block())
		list = nil
	}

	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if line.text == "" {
			continue
		}
		if line.breakBefore {
			flushParagraph()
			flushList()
		}
		if headingLevels[index] != 0 {
			flushParagraph()
			flushList()
			text := line.text
			links := slices.Clone(line.links)
			if index+1 < len(lines) &&
				isNumberedHeadingContinuation(
					line,
					lines[index+1],
					headingLevels[index],
					headingLevels[index+1],
				) {
				text, links = joinHeadingLines(text, links, lines[index+1])
				index++
			}
			blocks = append(blocks, &document.Heading{
				Level: headingLevels[index],
				Text:  text,
				Links: links,
			})
			continue
		}
		if item, ok := detectListItem(line.text); ok {
			flushParagraph()
			if list != nil && !list.accepts(item, line) {
				flushList()
			}
			if list == nil {
				list = newPendingList(item, line)
			} else {
				list.append(item, line)
			}
			continue
		}
		if list != nil {
			if list.acceptsContinuation(line) {
				list.appendContinuation(line)
				continue
			}
			flushList()
		}
		if len(paragraph) > 0 &&
			startsNewParagraph(paragraph[len(paragraph)-1], line, pageLeft, pageRight) {
			flushParagraph()
		}
		paragraph = append(paragraph, line)
	}
	flushParagraph()
	flushList()

	return blocks, nil
}

func isNumberedHeadingContinuation(
	current,
	next textLine,
	currentLevel,
	nextLevel int,
) bool {
	if currentLevel == 0 || currentLevel != nextLevel {
		return false
	}
	if _, numbered := numberedHeadingDepth(current.text); !numbered {
		return false
	}
	if _, numbered := numberedHeadingDepth(next.text); numbered {
		return false
	}
	scale := math.Max(current.scale(), next.scale())
	return scale > 0 &&
		next.top-current.bottom <= scale*paragraphGapRatio &&
		current.weight() == next.weight()
}

func joinHeadingLines(
	text string,
	links []document.TextLink,
	next textLine,
) (string, []document.TextLink) {
	offset := len(text) + 1
	text += " " + next.text
	for _, link := range next.links {
		link.Start += offset
		link.End += offset
		links = append(links, link)
	}
	return text, links
}

type detectedListItem struct {
	kind    document.ListKind
	ordinal int
	text    string
}

type pendingList struct {
	kind       document.ListKind
	start      int
	markerLeft float64
	markerSize float64
	items      [][]textLine
}

func newPendingList(item detectedListItem, line textLine) *pendingList {
	list := &pendingList{
		kind:       item.kind,
		start:      item.ordinal,
		markerLeft: line.left,
		markerSize: line.scale(),
	}
	list.append(item, line)
	return list
}

func (list *pendingList) accepts(item detectedListItem, line textLine) bool {
	if item.kind != list.kind {
		return false
	}
	alignmentTolerance := math.Max(list.markerSize, line.scale()) *
		listMarkerAlignRatio
	if math.Abs(line.left-list.markerLeft) > alignmentTolerance {
		return false
	}
	return item.kind != document.ListKindOrdered ||
		item.ordinal == list.start+len(list.items)
}

func (list *pendingList) acceptsContinuation(line textLine) bool {
	item := list.items[len(list.items)-1]
	previous := item[len(item)-1]
	lineHeight := math.Max(
		previous.bottom-previous.top,
		line.bottom-line.top,
	)
	if lineHeight <= 0 ||
		line.top-previous.bottom > lineHeight*paragraphGapRatio {
		return false
	}
	return line.left > list.markerLeft+lineHeight*paragraphIndentRatio
}

func (list *pendingList) append(item detectedListItem, line textLine) {
	line.setContent(item.text)
	list.items = append(list.items, []textLine{line})
}

func (list *pendingList) appendContinuation(line textLine) {
	last := len(list.items) - 1
	list.items[last] = append(list.items[last], line)
}

func (list *pendingList) block() *document.List {
	items := make([]document.ListItem, len(list.items))
	for index, lines := range list.items {
		text, links := joinWrappedContent(lines)
		items[index] = document.ListItem{Text: text, Links: links}
	}
	return &document.List{
		Kind:  list.kind,
		Start: list.start,
		Items: items,
	}
}

func (line *textLine) setContent(content string) {
	offset := strings.Index(line.text, content)
	if offset < 0 {
		line.text = content
		line.links = nil
		return
	}

	end := offset + len(content)
	links := line.links[:0]
	for _, link := range line.links {
		if link.End <= offset || link.Start >= end {
			continue
		}
		link.Start = max(link.Start, offset) - offset
		link.End = min(link.End, end) - offset
		links = append(links, link)
	}
	line.text = content
	line.links = links
}

func detectListItem(text string) (detectedListItem, bool) {
	first, firstSize := utf8.DecodeRuneInString(text)
	switch first {
	case '-', '*', '+', '\u2022', '\u25e6', '\u25aa':
		content, ok := listItemContent(text, firstSize)
		return detectedListItem{
			kind: document.ListKindUnordered,
			text: content,
		}, ok
	}

	digitEnd := 0
	for digitEnd < len(text) &&
		digitEnd < maximumListMarkerDigits &&
		text[digitEnd] >= '0' &&
		text[digitEnd] <= '9' {
		digitEnd++
	}
	if digitEnd == 0 || digitEnd >= len(text) ||
		text[digitEnd] != '.' && text[digitEnd] != ')' {
		return detectedListItem{}, false
	}
	content, ok := listItemContent(text, digitEnd+1)
	if !ok {
		return detectedListItem{}, false
	}
	ordinal, err := strconv.Atoi(text[:digitEnd])
	if err != nil {
		return detectedListItem{}, false
	}
	return detectedListItem{
		kind:    document.ListKindOrdered,
		ordinal: ordinal,
		text:    content,
	}, true
}

func listItemContent(text string, markerEnd int) (string, bool) {
	if markerEnd >= len(text) {
		return "", false
	}
	next, _ := utf8.DecodeRuneInString(text[markerEnd:])
	if !unicode.IsSpace(next) {
		return "", false
	}
	content := strings.TrimSpace(text[markerEnd:])
	return content, content != ""
}

func textMargins(lines []textLine, headingLevels []int) (float64, float64) {
	left := math.Inf(1)
	right := math.Inf(-1)
	for index, line := range lines {
		if line.text == "" || headingLevels[index] != 0 {
			continue
		}
		if _, ok := detectListItem(line.text); ok {
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
		numberedDepth, numbered := numberedHeadingDepth(line.text)
		if line.text == "" ||
			!hasSemanticHeadingText(line.text) ||
			hasDottedLeader(line.text) ||
			!numbered && lineLargeGapCount(line) > 0 ||
			!hasHeadingSpacing(lines, index, spacingThreshold) {
			continue
		}

		scaleRatio := line.scale() / bodyScale
		boldEvidence := line.weight() >= 600 &&
			line.weight() >= bodyWeight+200 &&
			hasStrongHeadingSpacing(lines, index, spacingThreshold)
		if scaleRatio < 1.15 && !boldEvidence {
			continue
		}
		if numbered {
			levels[index] = min(numberedDepth, 6)
			continue
		}
		if _, ok := detectListItem(line.text); ok {
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

func hasSemanticHeadingText(text string) bool {
	count := 0
	for _, value := range text {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			count++
			if count >= 2 {
				return true
			}
		}
	}
	return false
}

func numberedHeadingDepth(text string) (int, bool) {
	index := 0
	depth := 0
	for {
		start := index
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			index++
		}
		if index == start || index >= len(text) || text[index] != '.' {
			return 0, false
		}
		depth++
		index++
		if index >= len(text) || text[index] == ' ' || text[index] == '\t' {
			break
		}
	}
	if index >= len(text) {
		return 0, false
	}
	return depth, hasSemanticHeadingText(strings.TrimSpace(text[index:]))
}

func hasDottedLeader(text string) bool {
	return strings.Contains(text, "...")
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
	if current.breakBefore {
		return true
	}
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
	text, _ := joinWrappedContent(lines)
	return text
}

func joinWrappedContent(lines []textLine) (string, []document.TextLink) {
	var result strings.Builder
	var links []document.TextLink
	ambiguousColumns := hasAmbiguousColumns(lines)
	for index, line := range lines {
		text := line.text
		joinDiscretionaryBreak := index+1 < len(lines) &&
			!ambiguousColumns &&
			joinsDiscretionaryBreak(line, lines[index+1])
		if joinDiscretionaryBreak {
			text = strings.TrimSuffix(strings.TrimSuffix(text, "-"), "\u00ad")
		}
		offset := result.Len()
		for _, link := range line.links {
			if link.Start >= len(text) {
				continue
			}
			link.End = min(link.End, len(text))
			if link.End <= link.Start {
				continue
			}
			link.Start += offset
			link.End += offset
			links = append(links, link)
		}
		result.WriteString(text)
		if index+1 < len(lines) && !joinDiscretionaryBreak {
			result.WriteByte(' ')
		}
	}
	return result.String(), links
}

func hasAmbiguousColumns(lines []textLine) bool {
	return slices.ContainsFunc(lines, func(line textLine) bool {
		return lineLargeGapCount(line) > 0
	})
}

func joinsDiscretionaryBreak(current, next textLine) bool {
	return lineLargeGapCount(current) == 0 &&
		lineLargeGapCount(next) == 0 &&
		endsWithSoftHyphen(current.text, next.text)
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
