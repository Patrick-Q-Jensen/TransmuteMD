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
	maximumFieldWidthRatio  = 0.25
	maximumFieldHeightRatio = 0.1
	maximumFieldGapRatio    = 0.04
	contentsPageZoneRatio   = 0.7
	contentsAlignRatio      = 0.75
	minimumGeometricEntries = 2
	headingScaleTolerance   = 0.1
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

type blockRange struct {
	start int
	end   int
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
	allowContentsContinuation := false
	for _, page := range layout.Pages {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("analyze page %d: %w", page.Number, err)
		}

		lines, tableFallbacks, diagnostics, err := analyzePageLines(
			ctx,
			page,
			allowContentsContinuation,
		)
		if err != nil {
			return nil, fmt.Errorf("analyze page %d: %w", page.Number, err)
		}
		isContentsContinuation := allowContentsContinuation &&
			hasContentsEntries(lines) &&
			!hasContentsHeading(lines)
		allowContentsContinuation = contentsReachPageBottom(lines, page.Height)
		pages = append(pages, analyzedPage{
			page:                 page,
			lines:                lines,
			tableFallbacks:       tableFallbacks,
			diagnostics:          diagnostics,
			contentsContinuation: isContentsContinuation,
		})
	}
	if err := suppressRepeatedPageFurniture(ctx, pages); err != nil {
		return nil, fmt.Errorf("detect repeated page headers and footers: %w", err)
	}

	result := &document.Document{}
	result.Diagnostics = slices.Clone(layout.Diagnostics)
	pageBlocks := make([]blockRange, len(pages))
	for _, page := range pages {
		start := len(result.Blocks)
		result.Diagnostics = append(result.Diagnostics, page.diagnostics...)
		result.Diagnostics = append(
			result.Diagnostics,
			structureDiagnostics(
				page.page.Number,
				page.lines,
				page.tableFallbacks,
			)...,
		)
		blocks, err := groupBlocks(ctx, page.lines)
		if err != nil {
			return nil, fmt.Errorf("analyze page %d: %w", page.page.Number, err)
		}
		if page.contentsContinuation {
			blocks = mergeContentsContinuation(result.Blocks, blocks)
		}
		result.Blocks = append(result.Blocks, blocks...)
		pageBlocks[page.page.Number-1] = blockRange{
			start: start,
			end:   len(result.Blocks),
		}
	}
	if err := associateInternalNavigation(result, pageBlocks); err != nil {
		return nil, fmt.Errorf("associate internal navigation: %w", err)
	}

	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("validate analyzed document: %w", err)
	}
	return result, nil
}

type orderedRun struct {
	run        document.TextRun
	text       string
	index      int
	linkTarget document.LinkTarget
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
	inTable     bool
	table       *document.Table
	field       *document.Field
	contents    *detectedContentsEntry
}

type analyzedPage struct {
	page                 document.Page
	lines                []textLine
	tableFallbacks       []document.Rectangle
	diagnostics          []document.Diagnostic
	contentsContinuation bool
}

type detectedContentsEntry struct {
	number    string
	title     string
	page      int
	depth     int
	lineCount int
	target    document.LinkTarget
}

func contentsEntryTarget(lines []textLine) document.LinkTarget {
	var target document.LinkTarget
	for _, line := range lines {
		for _, link := range line.links {
			if link.Target.Kind == 0 {
				continue
			}
			if target.Kind != 0 && target != link.Target {
				return document.LinkTarget{}
			}
			target = link.Target
		}
	}
	return target
}

func analyzePageLines(
	ctx context.Context,
	page document.Page,
	allowContentsContinuation bool,
) ([]textLine, []document.Rectangle, []document.Diagnostic, error) {
	runs := make([]orderedRun, 0, len(page.TextRuns))
	ambiguousLink := false
	for index, run := range page.TextRuns {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}

		text := normalizeRunText(run)
		if text == "" {
			continue
		}
		target, ambiguous := linkTarget(run.Bounds, page.Links)
		ambiguousLink = ambiguousLink || ambiguous
		runs = append(runs, orderedRun{
			run:        run,
			text:       text,
			index:      index,
			linkTarget: target,
		})
	}

	detection, err := detectTables(ctx, page, runs)
	if err != nil {
		return nil, nil, nil, err
	}
	tables := detection.tables
	tableBlocks := make([]*document.Table, len(tables))
	for index, table := range tables {
		tableBlocks[index], err = semanticTable(
			ctx,
			table,
			page.TextPlaceholders,
		)
		if err != nil {
			return nil, nil, nil, err
		}
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
			return nil, nil, nil, err
		}
		if err := lines[index].finish(ctx); err != nil {
			return nil, nil, nil, err
		}
	}
	slices.SortStableFunc(lines, compareLines)
	lines = prepareContentsLines(lines, page.Width, allowContentsContinuation)
	lines = orderColumns(lines)
	for tableIndex, table := range tables {
		var tableLines []int
		safe := true
		for lineIndex, line := range lines {
			any, all := lineTableMembership(line, table)
			if !any {
				continue
			}
			tableLines = append(tableLines, lineIndex)
			safe = safe && all
		}
		if !safe || len(tableLines) == 0 {
			detection.rejected = append(detection.rejected, table.bounds)
			continue
		}
		for index, lineIndex := range tableLines {
			lines[lineIndex].inTable = true
			if index == 0 {
				lines[lineIndex].table = tableBlocks[tableIndex]
			}
		}
	}
	associateLabeledBlankFields(page, lines, tables)

	var diagnostics []document.Diagnostic
	if ambiguousLink {
		diagnostics = append(diagnostics, document.Diagnostic{
			Code:    document.DiagnosticAmbiguousLink,
			Page:    page.Number,
			Message: "overlapping link annotations were preserved as plain text",
		})
	}
	return lines, detection.rejected, diagnostics, nil
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

func linkTarget(
	bounds document.Rectangle,
	links []document.LinkAnnotation,
) (document.LinkTarget, bool) {
	centerX := (bounds.Left + bounds.Right) / 2
	centerY := (bounds.Top + bounds.Bottom) / 2
	var target document.LinkTarget
	for _, link := range links {
		if centerX < link.Bounds.Left || centerX > link.Bounds.Right ||
			centerY < link.Bounds.Top || centerY > link.Bounds.Bottom {
			continue
		}
		if target.Kind != 0 && target != link.Target {
			return document.LinkTarget{}, true
		}
		target = link.Target
	}
	return target, false
}

func structureDiagnostics(
	page int,
	lines []textLine,
	tableFallbacks []document.Rectangle,
) []document.Diagnostic {
	var diagnostics []document.Diagnostic
	var regions []document.Rectangle
	for _, region := range append(slices.Clone(tableFallbacks), tableLikeRegions(lines)...) {
		regions = addTableRegion(regions, region)
	}
	slices.SortStableFunc(regions, func(left, right document.Rectangle) int {
		if order := compareFloat(left.Top, right.Top); order != 0 {
			return order
		}
		return compareFloat(left.Left, right.Left)
	})
	for range regions {
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

func tableLikeRegions(lines []textLine) []document.Rectangle {
	var regions []document.Rectangle
	start := -1
	flush := func(end int) {
		if start < 0 || end-start < 3 {
			start = -1
			return
		}
		bounds := document.Rectangle{
			Left:   math.Inf(1),
			Top:    math.Inf(1),
			Right:  math.Inf(-1),
			Bottom: math.Inf(-1),
		}
		for _, line := range lines[start:end] {
			bounds.Left = math.Min(bounds.Left, line.left)
			bounds.Top = math.Min(bounds.Top, line.top)
			bounds.Right = math.Max(bounds.Right, line.right)
			bounds.Bottom = math.Max(bounds.Bottom, line.bottom)
		}
		regions = append(regions, bounds)
		start = -1
	}
	for index, line := range lines {
		if !line.inTable && line.contents == nil && lineLargeGapCount(line) >= 2 {
			if start < 0 {
				start = index
			}
			continue
		}
		flush(index)
	}
	flush(len(lines))
	return regions
}

func rectanglesOverlap(left, right document.Rectangle) bool {
	return math.Min(left.Right, right.Right) > math.Max(left.Left, right.Left) &&
		math.Min(left.Bottom, right.Bottom) > math.Max(left.Top, right.Top)
}

func addTableRegion(
	regions []document.Rectangle,
	candidate document.Rectangle,
) []document.Rectangle {
	for index := 0; index < len(regions); {
		if !rectanglesOverlap(regions[index], candidate) {
			index++
			continue
		}
		candidate = document.Rectangle{
			Left:   math.Min(candidate.Left, regions[index].Left),
			Top:    math.Min(candidate.Top, regions[index].Top),
			Right:  math.Max(candidate.Right, regions[index].Right),
			Bottom: math.Max(candidate.Bottom, regions[index].Bottom),
		}
		regions = slices.Delete(regions, index, index+1)
		index = 0
	}
	return append(regions, candidate)
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
	var activeLink document.LinkTarget
	activeStart := 0
	closeLink := func() {
		if activeLink.Kind == 0 {
			return
		}
		line.links = append(line.links, document.TextLink{
			Start:  activeStart,
			End:    result.Len(),
			Target: activeLink,
		})
		activeLink = document.LinkTarget{}
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
		if current.linkTarget != activeLink {
			closeLink()
		}
		if addSpace {
			result.WriteByte(' ')
		}
		if current.linkTarget.Kind != 0 && current.linkTarget != activeLink {
			activeLink = current.linkTarget
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
	index := (len(gaps) - 1) / 2
	if len(gaps) >= 6 {
		index = (len(gaps)*3 - 1) / 4
	}
	return gaps[index] * trackingGapMultiplier
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

type geometricContentsCandidate struct {
	titleIndex int
	consumed   int
	line       textLine
	entry      detectedContentsEntry
	pageRight  float64
}

func prepareContentsLines(
	lines []textLine,
	pageWidth float64,
	allowContinuation bool,
) []textLine {
	heading := -1
	for index, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line.text), "contents") {
			heading = index
			break
		}
	}
	if heading < 0 && !allowContinuation || pageWidth <= 0 {
		return lines
	}

	var candidates []geometricContentsCandidate
	started := false
	start := 0
	if heading >= 0 {
		start = heading + 1
	}
	for index := start; index < len(lines); {
		candidate, ok := geometricContentsCandidateAt(
			lines,
			index,
			pageWidth,
		)
		if !ok {
			if started {
				break
			}
			index++
			continue
		}
		started = true
		candidates = append(candidates, candidate)
		index += candidate.consumed
	}
	if len(candidates) < minimumGeometricEntries ||
		!alignedContentsPageNumbers(candidates) {
		return lines
	}

	hasRoot := false
	rootLeft := math.Inf(1)
	for _, candidate := range candidates {
		if candidate.entry.depth == 1 {
			hasRoot = true
			rootLeft = math.Min(rootLeft, candidate.line.left)
		}
	}
	if !hasRoot && !allowContinuation {
		return lines
	}
	if hasRoot {
		for _, candidate := range candidates {
			if candidate.entry.depth == 1 {
				continue
			}
			tolerance := candidate.line.scale() * listMarkerAlignRatio
			if candidate.line.left <= rootLeft+tolerance {
				return lines
			}
		}
	}

	byTitle := make(map[int]geometricContentsCandidate, len(candidates))
	skipped := make(map[int]struct{}, len(candidates)*2)
	for _, candidate := range candidates {
		byTitle[candidate.titleIndex] = candidate
		for offset := 1; offset < candidate.consumed; offset++ {
			skipped[candidate.titleIndex+offset] = struct{}{}
		}
	}

	result := make([]textLine, 0, len(lines)-len(skipped))
	for index, line := range lines {
		if _, skip := skipped[index]; skip {
			continue
		}
		candidate, ok := byTitle[index]
		if !ok {
			result = append(result, line)
			continue
		}
		entry := candidate.entry
		candidate.line.contents = &entry
		result = append(result, candidate.line)
	}
	return result
}

func hasContentsEntries(lines []textLine) bool {
	for _, line := range lines {
		if line.contents != nil {
			return true
		}
	}
	return false
}

func hasContentsHeading(lines []textLine) bool {
	for _, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line.text), "contents") {
			return true
		}
	}
	return false
}

func contentsReachPageBottom(lines []textLine, pageHeight float64) bool {
	if pageHeight <= 0 {
		return false
	}
	lastBottom := 0.0
	for _, line := range lines {
		if line.contents != nil {
			lastBottom = math.Max(lastBottom, line.bottom)
		}
	}
	return lastBottom >= pageHeight*(1-pageFurnitureBandRatio)
}

func geometricContentsCandidateAt(
	lines []textLine,
	index int,
	pageWidth float64,
) (geometricContentsCandidate, bool) {
	line := lines[index]
	if strings.Contains(line.text, "...") {
		return geometricContentsCandidate{}, false
	}

	title := ""
	page := 0
	pageRight := 0.0
	consumed := 1
	targetLines := []textLine{line}
	if sameLineTitle, sameLinePage, right, ok :=
		splitGeometricContentsPage(line, pageWidth); ok {
		title = sameLineTitle
		page = sameLinePage
		pageRight = right
	} else {
		if index+1 >= len(lines) {
			return geometricContentsCandidate{}, false
		}
		pageLine := lines[index+1]
		var ok bool
		page, ok = positivePageNumber(pageLine.text)
		if !ok ||
			pageLine.left < pageWidth*contentsPageZoneRatio ||
			!linesShareVerticalPosition(line, pageLine) ||
			!hasContentsPageGap(line.right, pageLine.left, line.scale(), pageWidth) {
			return geometricWrappedContentsCandidateAt(lines, index, pageWidth)
		}
		title = strings.TrimSpace(line.text)
		pageRight = pageLine.right
		consumed = 2
		targetLines = append(targetLines, pageLine)
		line.runs = append(line.runs, pageLine.runs...)
		line.top = math.Min(line.top, pageLine.top)
		line.bottom = math.Max(line.bottom, pageLine.bottom)
		line.right = math.Max(line.right, pageLine.right)
	}

	numberEnd, depth, ok := numberedPrefix(title)
	if !ok {
		return geometricContentsCandidate{}, false
	}
	entryTitle := strings.TrimSpace(title[numberEnd:])
	if !hasSemanticHeadingText(entryTitle) {
		return geometricContentsCandidate{}, false
	}
	entry := detectedContentsEntry{
		number:    strings.TrimSpace(title[:numberEnd]),
		title:     entryTitle,
		page:      page,
		depth:     depth,
		lineCount: 1,
		target:    contentsEntryTarget(targetLines),
	}
	line.text = entry.number + " " + entry.title
	return geometricContentsCandidate{
		titleIndex: index,
		consumed:   consumed,
		line:       line,
		entry:      entry,
		pageRight:  pageRight,
	}, true
}

func geometricWrappedContentsCandidateAt(
	lines []textLine,
	index int,
	pageWidth float64,
) (geometricContentsCandidate, bool) {
	if index+1 >= len(lines) {
		return geometricContentsCandidate{}, false
	}
	first := lines[index]
	firstText := strings.TrimSpace(first.text)
	numberEnd, depth, ok := numberedPrefix(firstText)
	if !ok {
		return geometricContentsCandidate{}, false
	}
	firstTitle := strings.TrimSpace(firstText[numberEnd:])
	second := lines[index+1]
	if !hasSemanticHeadingText(firstTitle) ||
		!hasSemanticHeadingText(second.text) ||
		strings.Contains(second.text, "...") {
		return geometricContentsCandidate{}, false
	}
	if _, _, numbered := numberedPrefix(strings.TrimSpace(second.text)); numbered {
		return geometricContentsCandidate{}, false
	}
	scale := math.Max(first.scale(), second.scale())
	if scale <= 0 ||
		second.top-first.bottom > scale*paragraphGapRatio ||
		second.left <= first.left+scale*listMarkerAlignRatio {
		return geometricContentsCandidate{}, false
	}

	secondTitle := ""
	page := 0
	pageRight := 0.0
	consumed := 2
	targetLines := []textLine{first, second}
	if title, sameLinePage, right, ok :=
		splitGeometricContentsPage(second, pageWidth); ok {
		secondTitle = title
		page = sameLinePage
		pageRight = right
	} else {
		if index+2 >= len(lines) {
			return geometricContentsCandidate{}, false
		}
		pageLine := lines[index+2]
		page, ok = positivePageNumber(pageLine.text)
		if !ok ||
			pageLine.left < pageWidth*contentsPageZoneRatio ||
			!linesShareVerticalPosition(second, pageLine) ||
			!hasContentsPageGap(
				second.right,
				pageLine.left,
				second.scale(),
				pageWidth,
			) {
			return geometricContentsCandidate{}, false
		}
		secondTitle = strings.TrimSpace(second.text)
		pageRight = pageLine.right
		consumed = 3
		targetLines = append(targetLines, pageLine)
		second.runs = append(second.runs, pageLine.runs...)
		second.top = math.Min(second.top, pageLine.top)
		second.bottom = math.Max(second.bottom, pageLine.bottom)
		second.right = math.Max(second.right, pageLine.right)
	}
	if !hasSemanticHeadingText(secondTitle) {
		return geometricContentsCandidate{}, false
	}

	line := first
	line.runs = append(line.runs, second.runs...)
	line.top = math.Min(line.top, second.top)
	line.bottom = math.Max(line.bottom, second.bottom)
	line.right = math.Max(line.right, second.right)
	entry := detectedContentsEntry{
		number:    strings.TrimSpace(firstText[:numberEnd]),
		title:     firstTitle + " " + secondTitle,
		page:      page,
		depth:     depth,
		lineCount: 1,
		target:    contentsEntryTarget(targetLines),
	}
	line.text = entry.number + " " + entry.title
	return geometricContentsCandidate{
		titleIndex: index,
		consumed:   consumed,
		line:       line,
		entry:      entry,
		pageRight:  pageRight,
	}, true
}

func splitGeometricContentsPage(
	line textLine,
	pageWidth float64,
) (string, int, float64, bool) {
	if len(line.runs) < 2 {
		return "", 0, 0, false
	}

	pageStart := len(line.runs)
	var pageParts []string
	for index := len(line.runs) - 1; index >= 0; index-- {
		text := strings.TrimSpace(line.runs[index].text)
		if text == "" {
			continue
		}
		if !allDigits(text) {
			break
		}
		pageStart = index
		pageParts = append(pageParts, text)
	}
	if pageStart <= 0 || pageStart >= len(line.runs) {
		return "", 0, 0, false
	}
	slices.Reverse(pageParts)
	pageText := strings.Join(pageParts, "")
	page, ok := positivePageNumber(pageText)
	if !ok {
		return "", 0, 0, false
	}

	pageRun := line.runs[pageStart].run
	titleRun := line.runs[pageStart-1].run
	if pageRun.Bounds.Left < pageWidth*contentsPageZoneRatio ||
		!hasContentsPageGap(
			titleRun.Bounds.Right,
			pageRun.Bounds.Left,
			line.scale(),
			pageWidth,
		) {
		return "", 0, 0, false
	}
	text := strings.TrimSpace(line.text)
	if !strings.HasSuffix(text, pageText) {
		return "", 0, 0, false
	}
	title := strings.TrimSpace(text[:len(text)-len(pageText)])
	if title == "" {
		return "", 0, 0, false
	}
	return title, page, line.runs[len(line.runs)-1].run.Bounds.Right, true
}

func positivePageNumber(text string) (int, bool) {
	text = strings.TrimSpace(text)
	if text == "" || !allDigits(text) {
		return 0, false
	}
	page, err := strconv.Atoi(text)
	return page, err == nil && page > 0
}

func allDigits(text string) bool {
	for index := range text {
		if text[index] < '0' || text[index] > '9' {
			return false
		}
	}
	return text != ""
}

func hasContentsPageGap(
	titleRight,
	pageLeft,
	scale,
	pageWidth float64,
) bool {
	return pageLeft-titleRight > math.Max(
		scale*columnGutterScaleRatio,
		pageWidth*columnGutterWidthRatio,
	)
}

func linesShareVerticalPosition(left, right textLine) bool {
	overlap := math.Min(left.bottom, right.bottom) - math.Max(left.top, right.top)
	height := math.Min(left.bottom-left.top, right.bottom-right.top)
	return height > 0 && overlap >= height*minimumLineOverlapRatio
}

func alignedContentsPageNumbers(candidates []geometricContentsCandidate) bool {
	rights := make([]float64, len(candidates))
	scales := make([]float64, 0, len(candidates))
	for index, candidate := range candidates {
		rights[index] = candidate.pageRight
		if scale := candidate.line.scale(); scale > 0 {
			scales = append(scales, scale)
		}
	}
	slices.Sort(rights)
	reference := rights[(len(rights)-1)/2]
	tolerance := 1.0
	if len(scales) > 0 {
		slices.Sort(scales)
		tolerance = math.Max(
			tolerance,
			scales[(len(scales)-1)/2]*contentsAlignRatio,
		)
	}
	for _, right := range rights {
		if math.Abs(right-reference) > tolerance {
			return false
		}
	}
	return true
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
	contentsEntries := detectContentsEntries(lines)
	headingLevels := detectHeadingLevels(lines, contentsEntries)
	pageLeft, pageRight := textMargins(lines, headingLevels, contentsEntries)
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
		if line.breakBefore {
			flushParagraph()
			flushList()
		}
		if line.inTable {
			flushParagraph()
			flushList()
			if line.table != nil {
				blocks = append(blocks, line.table)
			}
			continue
		}
		if line.field != nil {
			flushParagraph()
			flushList()
			blocks = append(blocks, line.field)
			continue
		}
		if line.text == "" {
			continue
		}
		if contentsEntries[index] != nil {
			flushParagraph()
			flushList()
			contents, next := buildContentsList(contentsEntries, index)
			blocks = append(blocks, contents)
			index = next - 1
			continue
		}
		if headingLevels[index] != 0 {
			flushParagraph()
			flushList()
			level := headingLevels[index]
			text := line.text
			links := slices.Clone(line.links)
			if index+1 < len(lines) &&
				contentsEntries[index+1] == nil &&
				isNumberedHeadingContinuation(
					line,
					lines[index+1],
					level,
					headingLevels[index+1],
				) {
				text, links = joinHeadingLines(text, links, lines[index+1])
				index++
			}
			blocks = append(blocks, &document.Heading{
				Level: level,
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

func buildContentsList(
	entries []*detectedContentsEntry,
	start int,
) (*document.List, int) {
	var flat []detectedContentsEntry
	index := start
	for index < len(entries) {
		entry := entries[index]
		if entry == nil {
			break
		}
		flat = append(flat, *entry)
		index += entry.lineCount
	}
	rootDepth := flat[0].depth
	for _, entry := range flat[1:] {
		rootDepth = min(rootDepth, entry.depth)
	}
	items, _ := buildContentsItems(flat, 0, rootDepth)
	return &document.List{
		Kind:  document.ListKindUnordered,
		Items: items,
	}, index
}

func mergeContentsContinuation(
	previous,
	current []document.Block,
) []document.Block {
	if len(previous) == 0 || len(current) == 0 {
		return current
	}
	target, ok := previous[len(previous)-1].(*document.List)
	if !ok || target.Kind != document.ListKindUnordered {
		return current
	}
	continuation, ok := current[0].(*document.List)
	if !ok || continuation.Kind != document.ListKindUnordered {
		return current
	}

	items := flattenContentsItems(continuation.Items)
	merged := *target
	merged.Items = cloneContentsItems(target.Items)
	for _, item := range items {
		_, depth, ok := numberedPrefix(item.Text)
		if !ok || !appendContentsItemAtDepth(&merged, item, depth) {
			return current
		}
	}
	*target = merged
	return current[1:]
}

func cloneContentsItems(items []document.ListItem) []document.ListItem {
	result := make([]document.ListItem, len(items))
	for index, item := range items {
		result[index] = item
		result[index].Links = slices.Clone(item.Links)
		result[index].Children = make([]document.List, len(item.Children))
		for childIndex, child := range item.Children {
			result[index].Children[childIndex] = child
			result[index].Children[childIndex].Items = cloneContentsItems(child.Items)
		}
	}
	return result
}

func flattenContentsItems(items []document.ListItem) []document.ListItem {
	var result []document.ListItem
	for _, item := range items {
		children := item.Children
		item.Children = nil
		result = append(result, item)
		for _, child := range children {
			result = append(result, flattenContentsItems(child.Items)...)
		}
	}
	return result
}

func appendContentsItemAtDepth(
	list *document.List,
	item document.ListItem,
	depth int,
) bool {
	if depth < 1 {
		return false
	}
	if depth == 1 {
		list.Items = append(list.Items, item)
		return true
	}

	current := list
	for currentDepth := 1; currentDepth < depth; currentDepth++ {
		if len(current.Items) == 0 {
			return false
		}
		parent := &current.Items[len(current.Items)-1]
		if currentDepth == depth-1 {
			if len(parent.Children) == 0 {
				parent.Children = append(parent.Children, document.List{
					Kind: document.ListKindUnordered,
				})
			}
			child := &parent.Children[len(parent.Children)-1]
			if child.Kind != document.ListKindUnordered {
				return false
			}
			child.Items = append(child.Items, item)
			return true
		}
		if len(parent.Children) == 0 {
			return false
		}
		current = &parent.Children[len(parent.Children)-1]
		if current.Kind != document.ListKindUnordered {
			return false
		}
	}
	return false
}

func buildContentsItems(
	entries []detectedContentsEntry,
	start,
	depth int,
) ([]document.ListItem, int) {
	var items []document.ListItem
	index := start
	for index < len(entries) {
		entry := entries[index]
		if entry.depth < depth {
			break
		}
		if entry.depth > depth {
			if len(items) == 0 {
				items = append(items, contentsListItem(entry))
				index++
				continue
			}
			children, next := buildContentsItems(entries, index, entry.depth)
			last := len(items) - 1
			items[last].Children = append(items[last].Children, document.List{
				Kind:  document.ListKindUnordered,
				Items: children,
			})
			index = next
			continue
		}
		items = append(items, contentsListItem(entry))
		index++
	}
	return items, index
}

func contentsListItem(entry detectedContentsEntry) document.ListItem {
	item := document.ListItem{Text: entry.number + " " + entry.title}
	if entry.target.Kind != 0 {
		item.Links = []document.TextLink{
			{
				Start:  0,
				End:    len(item.Text),
				Target: entry.target,
			},
		}
	}
	return item
}

func associateInternalNavigation(
	doc *document.Document,
	pageBlocks []blockRange,
) error {
	headingAnchors := allocateHeadingAnchors(doc.Blocks)
	return visitBlockLinks(doc.Blocks, func(text string, link *document.TextLink) {
		heading := linkedHeading(text, *link, doc.Blocks, pageBlocks)
		if heading == nil {
			return
		}
		anchor := headingAnchors[heading]
		heading.Anchor = anchor
		link.Target = document.LinkTarget{
			Kind: document.LinkTargetNamed,
			Name: anchor,
		}
	})
}

func allocateHeadingAnchors(
	blocks []document.Block,
) map[*document.Heading]string {
	result := make(map[*document.Heading]string)
	counts := make(map[string]int)
	for _, block := range blocks {
		heading, ok := block.(*document.Heading)
		if !ok {
			continue
		}
		base := headingAnchor(heading.Text)
		counts[base]++
		anchor := base
		if counts[base] > 1 {
			anchor += "-" + strconv.Itoa(counts[base])
		}
		result[heading] = anchor
	}
	return result
}

func headingAnchor(text string) string {
	var result strings.Builder
	pendingHyphen := false
	for _, value := range strings.ToLower(text) {
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			if pendingHyphen && result.Len() > 0 {
				result.WriteByte('-')
			}
			result.WriteRune(value)
			pendingHyphen = false
			continue
		}
		pendingHyphen = result.Len() > 0
	}
	if result.Len() == 0 {
		return "section"
	}
	return result.String()
}

func visitBlockLinks(
	blocks []document.Block,
	visit func(string, *document.TextLink),
) error {
	for _, block := range blocks {
		switch typed := block.(type) {
		case *document.Field:
			visitTextLinks(typed.Label, typed.Links, visit)
		case *document.Paragraph:
			visitTextLinks(typed.Text, typed.Links, visit)
		case *document.Heading:
			visitTextLinks(typed.Text, typed.Links, visit)
		case *document.List:
			visitListLinks(typed, visit)
		case *document.Table:
			for rowIndex := range typed.Rows {
				for cellIndex := range typed.Rows[rowIndex].Cells {
					cell := &typed.Rows[rowIndex].Cells[cellIndex]
					visitTextLinks(cell.Text, cell.Links, visit)
				}
			}
		default:
			return fmt.Errorf("unsupported block type %T", block)
		}
	}
	return nil
}

func visitListLinks(
	list *document.List,
	visit func(string, *document.TextLink),
) {
	for itemIndex := range list.Items {
		item := &list.Items[itemIndex]
		visitTextLinks(item.Text, item.Links, visit)
		for childIndex := range item.Children {
			visitListLinks(&item.Children[childIndex], visit)
		}
	}
}

func visitTextLinks(
	text string,
	links []document.TextLink,
	visit func(string, *document.TextLink),
) {
	for index := range links {
		visit(text, &links[index])
	}
}

func linkedHeading(
	text string,
	link document.TextLink,
	blocks []document.Block,
	pageBlocks []blockRange,
) *document.Heading {
	label := normalizeNavigationLabel(text[link.Start:link.End])
	switch link.Target.Kind {
	case document.LinkTargetPage:
		page := link.Target.Page
		if page < 1 || page > len(pageBlocks) {
			return nil
		}
		scope := pageBlocks[page-1]
		return matchingHeading(label, blocks[scope.start:scope.end])
	case document.LinkTargetNamed:
		return matchingNamedHeading(link.Target.Name, blocks)
	default:
		return nil
	}
}

func matchingNamedHeading(
	name string,
	blocks []document.Block,
) *document.Heading {
	normalized := normalizeNavigationLabel(name)
	for _, block := range blocks {
		heading, ok := block.(*document.Heading)
		if !ok {
			continue
		}
		if headingAnchor(heading.Text) == name ||
			normalizeNavigationLabel(heading.Text) == normalized {
			return heading
		}
	}
	return nil
}

func matchingHeading(
	label string,
	blocks []document.Block,
) *document.Heading {
	var only *document.Heading
	count := 0
	for _, block := range blocks {
		heading, ok := block.(*document.Heading)
		if !ok {
			continue
		}
		count++
		only = heading
		if normalizeNavigationLabel(heading.Text) == label {
			return heading
		}
	}
	if count == 1 {
		return only
	}
	return nil
}

func normalizeNavigationLabel(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

func isNumberedHeadingContinuation(
	current,
	next textLine,
	currentLevel,
	nextLevel int,
) bool {
	if currentLevel == 0 ||
		nextLevel != 0 && currentLevel != nextLevel ||
		next.breakBefore ||
		next.inTable ||
		next.field != nil ||
		!hasSemanticHeadingText(next.text) {
		return false
	}
	if _, numbered := numberedHeadingDepth(current.text); !numbered {
		return false
	}
	if _, numbered := numberedHeadingDepth(next.text); numbered {
		return false
	}
	if _, listItem := detectListItem(next.text); listItem {
		return false
	}
	currentScale := current.scale()
	nextScale := next.scale()
	scale := math.Max(currentScale, nextScale)
	return currentScale > 0 &&
		nextScale > 0 &&
		math.Abs(currentScale-nextScale) <= scale*headingScaleTolerance &&
		next.top >= current.top &&
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

func textMargins(
	lines []textLine,
	headingLevels []int,
	contentsEntries []*detectedContentsEntry,
) (float64, float64) {
	left := math.Inf(1)
	right := math.Inf(-1)
	for index, line := range lines {
		if line.text == "" ||
			headingLevels[index] != 0 ||
			contentsEntries[index] != nil {
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

func detectHeadingLevels(
	lines []textLine,
	contentsEntries []*detectedContentsEntry,
) []int {
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
			contentsEntries[index] != nil ||
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
	end, depth, ok := numberedPrefix(text)
	if !ok {
		return 0, false
	}
	return depth, hasSemanticHeadingText(strings.TrimSpace(text[end:]))
}

func numberedPrefix(text string) (int, int, bool) {
	index := 0
	depth := 0
	for {
		start := index
		for index < len(text) && text[index] >= '0' && text[index] <= '9' {
			index++
		}
		if index == start || index >= len(text) || text[index] != '.' {
			return 0, 0, false
		}
		depth++
		index++
		if index >= len(text) || text[index] == ' ' || text[index] == '\t' {
			break
		}
	}
	if index >= len(text) {
		return 0, 0, false
	}
	return index, depth, true
}

func detectContentsEntries(lines []textLine) []*detectedContentsEntry {
	entries := make([]*detectedContentsEntry, len(lines))
	contentsHeading := -1
	hasPreparedEntries := false
	for index, line := range lines {
		hasPreparedEntries = hasPreparedEntries || line.contents != nil
		if strings.EqualFold(strings.TrimSpace(line.text), "contents") {
			contentsHeading = index
			break
		}
	}
	if contentsHeading < 0 && !hasPreparedEntries {
		return entries
	}

	rootLeft := math.Inf(1)
	start := 0
	if contentsHeading >= 0 {
		start = contentsHeading + 1
	}
	for index := start; index < len(lines); index++ {
		var entry detectedContentsEntry
		ok := false
		if lines[index].contents != nil {
			entry = *lines[index].contents
			ok = true
		} else {
			entry, ok = detectContentsEntry(lines[index].text)
			if !ok && index+1 < len(lines) {
				entry, ok = detectWrappedContentsEntry(lines[index], lines[index+1])
			}
		}
		if !ok {
			continue
		}
		if entry.target.Kind == 0 {
			entry.target = contentsEntryTarget(lines[index : index+entry.lineCount])
		}
		entries[index] = &entry
		if entry.depth == 1 {
			rootLeft = math.Min(rootLeft, lines[index].left)
		}
		index += entry.lineCount - 1
	}
	if math.IsInf(rootLeft, 1) && !hasPreparedEntries {
		return make([]*detectedContentsEntry, len(lines))
	}

	for index, entry := range entries {
		if entry == nil ||
			entry.depth == 1 ||
			lines[index].contents != nil ||
			math.IsInf(rootLeft, 1) {
			continue
		}
		tolerance := lines[index].scale() * listMarkerAlignRatio
		if lines[index].left <= rootLeft+tolerance {
			entries[index] = nil
		}
	}
	return entries
}

func detectContentsEntry(text string) (detectedContentsEntry, bool) {
	text = strings.TrimSpace(text)
	numberEnd, depth, ok := numberedPrefix(text)
	if !ok {
		return detectedContentsEntry{}, false
	}
	title, page, ok := detectContentsEntryTail(text[numberEnd:])
	if !ok {
		return detectedContentsEntry{}, false
	}
	return detectedContentsEntry{
		number:    strings.TrimSpace(text[:numberEnd]),
		title:     title,
		page:      page,
		depth:     depth,
		lineCount: 1,
	}, true
}

func detectWrappedContentsEntry(
	first,
	second textLine,
) (detectedContentsEntry, bool) {
	text := strings.TrimSpace(first.text)
	numberEnd, depth, ok := numberedPrefix(text)
	if !ok || strings.Contains(text[numberEnd:], "...") {
		return detectedContentsEntry{}, false
	}
	firstTitle := strings.TrimSpace(text[numberEnd:])
	secondTitle, page, ok := detectContentsEntryTail(second.text)
	scale := math.Max(first.scale(), second.scale())
	if !ok ||
		!hasSemanticHeadingText(firstTitle) ||
		scale <= 0 ||
		second.top-first.bottom > scale*paragraphGapRatio ||
		second.left <= first.left+scale*listMarkerAlignRatio {
		return detectedContentsEntry{}, false
	}
	return detectedContentsEntry{
		number:    strings.TrimSpace(text[:numberEnd]),
		title:     firstTitle + " " + secondTitle,
		page:      page,
		depth:     depth,
		lineCount: 2,
	}, true
}

func detectContentsEntryTail(text string) (string, int, bool) {
	text = strings.TrimSpace(text)
	leader := strings.Index(text, "...")
	if leader <= 0 {
		return "", 0, false
	}
	title := strings.TrimSpace(text[:leader])
	pageText := strings.Trim(strings.TrimSpace(text[leader:]), ". ")
	if !hasSemanticHeadingText(title) || pageText == "" {
		return "", 0, false
	}
	for _, value := range pageText {
		if !unicode.IsDigit(value) {
			return "", 0, false
		}
	}
	page, err := strconv.Atoi(pageText)
	if err != nil || page < 1 {
		return "", 0, false
	}
	return title, page, true
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
