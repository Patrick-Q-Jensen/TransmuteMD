package analyze

import (
	"context"
	"math"
	"slices"
	"strings"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

const (
	tableCoordinateTolerance         = 2.5
	minimumTableCellHeight           = 4.0
	minimumTableCellWidth            = 8.0
	minimumHorizontalTableBodyRows   = 1
	minimumRowsBeforeTableTruncation = 2
	horizontalAnchorBodySupportRatio = 0.75
	maximumFirstAnchorInset          = 8.0
	horizontalRowContinuationRatio   = 1.5
)

type tableSegment struct {
	position float64
	start    float64
	end      float64
}

type detectedTable struct {
	bounds             document.Rectangle
	columns            []float64
	rows               [][]detectedTableCell
	requireHeaderStyle bool
	horizontal         bool
}

type detectedTableCell struct {
	bounds document.Rectangle
	runs   []orderedRun
}

type tableDetection struct {
	tables   []detectedTable
	rejected []document.Rectangle
}

type tableCellContent struct {
	text  string
	links []document.TextLink
}

type horizontalSegmentGroup struct {
	left  float64
	right float64
	lines []tableSegment
}

type tableRowBand struct {
	top     float64
	bottom  float64
	columns []float64
}

type tableBandGroup struct {
	bands   []tableRowBand
	columns []float64
}

func detectTables(
	ctx context.Context,
	page document.Page,
	runs []orderedRun,
) (tableDetection, error) {
	horizontal, vertical := normalizedTableSegments(page.Rulings)
	groups := groupHorizontalSegments(horizontal)
	var result tableDetection
	for _, group := range groups {
		if err := ctx.Err(); err != nil {
			return tableDetection{}, err
		}
		if len(group.lines) == 2 {
			bandGroups := tableBandGroups(group, vertical)
			if len(bandGroups) != 1 {
				continue
			}
			bounds := tableBandGroupBounds(group, bandGroups[0])
			if isPageFurnitureTable(bounds, page.Height) {
				continue
			}
			table, ok, err := tableFromStackedFieldBand(
				ctx,
				group,
				bandGroups[0],
				runs,
			)
			if err != nil {
				return tableDetection{}, err
			}
			if ok {
				result.tables = append(result.tables, table)
			}
			continue
		}
		bounds, candidate := tableCandidateBounds(group)
		if !candidate || isPageFurnitureTable(bounds, page.Height) {
			continue
		}
		bandGroups := tableBandGroups(group, vertical)
		for _, bandGroup := range bandGroups {
			bandBounds := tableBandGroupBounds(group, bandGroup)
			table, ok := tableFromBandGroup(
				group,
				bandGroup,
				len(bandGroups) > 1,
			)
			if ok {
				if !assignTableRuns(&table, runs) ||
					table.requireHeaderStyle && !hasReliableTableHeader(table) {
					result.rejected = append(result.rejected, table.bounds)
					continue
				}
				result.tables = append(result.tables, table)
				continue
			}
			table, ok, stackedErr := tableFromStackedFieldBand(
				ctx,
				group,
				bandGroup,
				runs,
			)
			if stackedErr != nil {
				return tableDetection{}, stackedErr
			}
			if ok {
				result.tables = append(result.tables, table)
				continue
			}
			if len(bandGroup.columns) == 2 {
				inferred, inferredRejected, inferErr := inferredHorizontalTables(
					ctx,
					group,
					bandGroup,
					runs,
				)
				if inferErr != nil {
					return tableDetection{}, inferErr
				}
				result.rejected = append(result.rejected, inferredRejected...)
				if len(inferred) > 0 {
					result.tables = append(result.tables, inferred...)
					rejected, rejectErr := horizontalTableFallbacks(
						ctx,
						bandBounds,
						inferred,
						runs,
					)
					if rejectErr != nil {
						return tableDetection{}, rejectErr
					}
					result.rejected = append(result.rejected, rejected...)
					continue
				}
				tableLike, err := hasTableCandidateText(ctx, bandBounds, runs)
				if err != nil {
					return tableDetection{}, err
				}
				if tableLike {
					result.rejected = append(result.rejected, bandBounds)
				}
				continue
			}
			tableLike, err := hasTableCandidateText(ctx, bandBounds, runs)
			if err != nil {
				return tableDetection{}, err
			}
			if tableLike {
				result.rejected = append(result.rejected, bandBounds)
			}
		}
		if len(bandGroups) == 0 {
			tableLike, err := hasTableCandidateText(ctx, bounds, runs)
			if err != nil {
				return tableDetection{}, err
			}
			if tableLike {
				result.rejected = append(result.rejected, bounds)
			}
		}
	}
	slices.SortStableFunc(result.tables, func(left, right detectedTable) int {
		if order := compareFloat(left.bounds.Top, right.bounds.Top); order != 0 {
			return order
		}
		return compareFloat(left.bounds.Left, right.bounds.Left)
	})
	slices.SortStableFunc(result.rejected, func(left, right document.Rectangle) int {
		if order := compareFloat(left.Top, right.Top); order != 0 {
			return order
		}
		return compareFloat(left.Left, right.Left)
	})
	return result, nil
}

func inferredHorizontalTables(
	ctx context.Context,
	group horizontalSegmentGroup,
	bandGroup tableBandGroup,
	runs []orderedRun,
) ([]detectedTable, []document.Rectangle, error) {
	var tables []detectedTable
	var rejected []document.Rectangle
	for start := 0; start < len(bandGroup.bands)-1; {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if !tableBandHasBoldText(group, bandGroup.bands[start], runs) {
			start++
			continue
		}

		end := len(bandGroup.bands)
		for index := start + 1; index < len(bandGroup.bands); index++ {
			if tableBandHasBoldText(group, bandGroup.bands[index], runs) {
				end = index
				break
			}
		}
		if end-start < minimumHorizontalTableBodyRows+1 {
			start++
			continue
		}

		candidate := tableBandGroup{
			bands:   slices.Clone(bandGroup.bands[start:end]),
			columns: bandGroup.columns,
		}
		table, ok, err := inferredHorizontalTable(ctx, group, candidate, runs)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			start++
			continue
		}
		assigned, assignErr := assignHorizontalTableRuns(ctx, &table, runs)
		if assignErr != nil {
			return nil, nil, assignErr
		}
		if assigned && hasDistinctTableHeader(table) {
			tables = append(tables, table)
			start = end
			continue
		}
		rejected = append(rejected, table.bounds)
		start++
	}
	return tables, rejected, nil
}

func tableBandHasBoldText(
	group horizontalSegmentGroup,
	band tableRowBand,
	runs []orderedRun,
) bool {
	var weights []int
	for _, run := range runs {
		centerX := (run.run.Bounds.Left + run.run.Bounds.Right) / 2
		centerY := verticalCenter(run.run.Bounds)
		if centerX <= group.left ||
			centerX >= group.right ||
			centerY <= band.top ||
			centerY >= band.bottom ||
			strings.TrimSpace(run.text) == "" {
			continue
		}
		weights = append(weights, run.run.Style.FontWeight)
	}
	if len(weights) == 0 {
		return false
	}
	slices.Sort(weights)
	return weights[(len(weights)-1)/2] >= 600
}

func horizontalTableFallbacks(
	ctx context.Context,
	bounds document.Rectangle,
	tables []detectedTable,
	runs []orderedRun,
) ([]document.Rectangle, error) {
	cursor := bounds.Top
	var rejected []document.Rectangle
	for _, table := range tables {
		if table.bounds.Top > cursor+tableCoordinateTolerance {
			gap := bounds
			gap.Top = cursor
			gap.Bottom = table.bounds.Top
			tableLike, err := hasTableCandidateText(ctx, gap, runs)
			if err != nil {
				return nil, err
			}
			if tableLike {
				rejected = append(rejected, gap)
			}
		}
		cursor = math.Max(cursor, table.bounds.Bottom)
	}
	if cursor < bounds.Bottom-tableCoordinateTolerance {
		gap := bounds
		gap.Top = cursor
		tableLike, err := hasTableCandidateText(ctx, gap, runs)
		if err != nil {
			return nil, err
		}
		if tableLike {
			rejected = append(rejected, gap)
		}
	}
	return rejected, nil
}

func tableCandidateBounds(
	group horizontalSegmentGroup,
) (document.Rectangle, bool) {
	if len(group.lines) < 3 {
		return document.Rectangle{}, false
	}
	top := math.Inf(1)
	bottom := math.Inf(-1)
	for _, line := range group.lines {
		top = math.Min(top, line.position)
		bottom = math.Max(bottom, line.position)
	}
	if bottom-top < minimumTableCellHeight*2 {
		return document.Rectangle{}, false
	}
	return document.Rectangle{
		Left:   group.left,
		Top:    top,
		Right:  group.right,
		Bottom: bottom,
	}, true
}

func isPageFurnitureTable(bounds document.Rectangle, pageHeight float64) bool {
	return bounds.Bottom <= pageHeight*pageFurnitureBandRatio ||
		bounds.Top >= pageHeight*(1-pageFurnitureBandRatio)
}

func hasTableCandidateText(
	ctx context.Context,
	bounds document.Rectangle,
	runs []orderedRun,
) (bool, error) {
	var contained []orderedRun
	for _, run := range runs {
		centerX := (run.run.Bounds.Left + run.run.Bounds.Right) / 2
		centerY := verticalCenter(run.run.Bounds)
		if centerX > bounds.Left &&
			centerX < bounds.Right &&
			centerY > bounds.Top &&
			centerY < bounds.Bottom {
			contained = append(contained, run)
		}
	}
	lines, err := tableCellLines(ctx, contained)
	if err != nil {
		return false, err
	}
	gapped := 0
	for _, line := range lines {
		if lineLargeGapCount(line) > 0 {
			gapped++
		}
	}
	return gapped >= 2, nil
}

func normalizedTableSegments(
	rulings []document.Ruling,
) ([]tableSegment, []tableSegment) {
	horizontal := make([]tableSegment, 0, len(rulings))
	vertical := make([]tableSegment, 0, len(rulings))
	for _, ruling := range rulings {
		if ruling.Start.Y == ruling.End.Y {
			horizontal = append(horizontal, tableSegment{
				position: ruling.Start.Y,
				start:    math.Min(ruling.Start.X, ruling.End.X),
				end:      math.Max(ruling.Start.X, ruling.End.X),
			})
			continue
		}
		vertical = append(vertical, tableSegment{
			position: ruling.Start.X,
			start:    math.Min(ruling.Start.Y, ruling.End.Y),
			end:      math.Max(ruling.Start.Y, ruling.End.Y),
		})
	}
	return mergeTableSegments(horizontal), mergeTableSegments(vertical)
}

func mergeTableSegments(segments []tableSegment) []tableSegment {
	slices.SortStableFunc(segments, func(left, right tableSegment) int {
		if order := compareFloat(left.position, right.position); order != 0 {
			return order
		}
		if order := compareFloat(left.start, right.start); order != 0 {
			return order
		}
		return compareFloat(left.end, right.end)
	})

	var merged []tableSegment
	for start := 0; start < len(segments); {
		end := start + 1
		position := segments[start].position
		for end < len(segments) &&
			math.Abs(segments[end].position-position) <= tableCoordinateTolerance {
			position = (position*float64(end-start) + segments[end].position) /
				float64(end-start+1)
			end++
		}

		cluster := slices.Clone(segments[start:end])
		slices.SortStableFunc(cluster, func(left, right tableSegment) int {
			if order := compareFloat(left.start, right.start); order != 0 {
				return order
			}
			return compareFloat(left.end, right.end)
		})
		current := tableSegment{
			position: position,
			start:    cluster[0].start,
			end:      cluster[0].end,
		}
		for _, segment := range cluster[1:] {
			if segment.start <= current.end+tableCoordinateTolerance {
				current.end = math.Max(current.end, segment.end)
				continue
			}
			merged = append(merged, current)
			current = tableSegment{
				position: position,
				start:    segment.start,
				end:      segment.end,
			}
		}
		merged = append(merged, current)
		start = end
	}
	return merged
}

func groupHorizontalSegments(
	horizontal []tableSegment,
) []horizontalSegmentGroup {
	var groups []horizontalSegmentGroup
	for _, segment := range horizontal {
		if segment.end-segment.start < minimumTableCellWidth*2 {
			continue
		}
		groupIndex := -1
		for index := range groups {
			if math.Abs(groups[index].left-segment.start) <= tableCoordinateTolerance &&
				math.Abs(groups[index].right-segment.end) <= tableCoordinateTolerance {
				groupIndex = index
				break
			}
		}
		if groupIndex < 0 {
			groups = append(groups, horizontalSegmentGroup{
				left:  segment.start,
				right: segment.end,
				lines: []tableSegment{segment},
			})
			continue
		}
		group := &groups[groupIndex]
		count := float64(len(group.lines))
		group.left = (group.left*count + segment.start) / (count + 1)
		group.right = (group.right*count + segment.end) / (count + 1)
		group.lines = append(group.lines, segment)
	}
	return groups
}

func tableBandGroups(
	group horizontalSegmentGroup,
	vertical []tableSegment,
) []tableBandGroup {
	if len(group.lines) < 2 {
		return nil
	}
	slices.SortStableFunc(group.lines, func(left, right tableSegment) int {
		return compareFloat(left.position, right.position)
	})

	var bands []tableRowBand
	for index := 1; index < len(group.lines); index++ {
		top := group.lines[index-1].position
		bottom := group.lines[index].position
		if bottom-top < minimumTableCellHeight {
			continue
		}
		rowColumns := []float64{group.left}
		for _, segment := range vertical {
			if segment.position <= group.left+tableCoordinateTolerance ||
				segment.position >= group.right-tableCoordinateTolerance ||
				segment.start > top+tableCoordinateTolerance ||
				segment.end < bottom-tableCoordinateTolerance {
				continue
			}
			rowColumns = append(rowColumns, segment.position)
		}
		rowColumns = append(rowColumns, group.right)
		rowColumns = mergeCoordinates(rowColumns)
		if !validColumnWidths(rowColumns) {
			continue
		}
		bands = append(bands, tableRowBand{
			top:     top,
			bottom:  bottom,
			columns: rowColumns,
		})
	}

	var groups []tableBandGroup
	for _, band := range bands {
		if len(groups) == 0 {
			groups = append(groups, tableBandGroup{
				bands:   []tableRowBand{band},
				columns: band.columns,
			})
			continue
		}
		current := &groups[len(groups)-1]
		previous := current.bands[len(current.bands)-1]
		if !sameCoordinates(current.columns, band.columns) ||
			math.Abs(previous.bottom-band.top) > tableCoordinateTolerance {
			groups = append(groups, tableBandGroup{
				bands:   []tableRowBand{band},
				columns: band.columns,
			})
			continue
		}
		current.bands = append(current.bands, band)
	}
	return groups
}

func tableBandGroupBounds(
	group horizontalSegmentGroup,
	bandGroup tableBandGroup,
) document.Rectangle {
	return document.Rectangle{
		Left:   group.left,
		Top:    bandGroup.bands[0].top,
		Right:  group.right,
		Bottom: bandGroup.bands[len(bandGroup.bands)-1].bottom,
	}
}

func tableFromBandGroup(
	group horizontalSegmentGroup,
	bandGroup tableBandGroup,
	segmented bool,
) (detectedTable, bool) {
	if len(bandGroup.bands) < 2 || len(bandGroup.columns) < 3 {
		return detectedTable{}, false
	}
	rows := make([][]detectedTableCell, len(bandGroup.bands))
	for rowIndex := range rows {
		rows[rowIndex] = make([]detectedTableCell, len(bandGroup.columns)-1)
		for columnIndex := range rows[rowIndex] {
			rows[rowIndex][columnIndex].bounds = document.Rectangle{
				Left:   bandGroup.columns[columnIndex],
				Top:    bandGroup.bands[rowIndex].top,
				Right:  bandGroup.columns[columnIndex+1],
				Bottom: bandGroup.bands[rowIndex].bottom,
			}
		}
	}
	return detectedTable{
		bounds:             tableBandGroupBounds(group, bandGroup),
		columns:            bandGroup.columns,
		rows:               rows,
		requireHeaderStyle: segmented,
	}, true
}

func tableFromStackedFieldBand(
	ctx context.Context,
	group horizontalSegmentGroup,
	bandGroup tableBandGroup,
	runs []orderedRun,
) (detectedTable, bool, error) {
	if len(bandGroup.bands) != 1 || len(bandGroup.columns) < 3 {
		return detectedTable{}, false, nil
	}
	band := bandGroup.bands[0]
	cellRuns := make([][]orderedRun, len(bandGroup.columns)-1)
	for _, run := range runs {
		centerX := (run.run.Bounds.Left + run.run.Bounds.Right) / 2
		centerY := verticalCenter(run.run.Bounds)
		if centerX <= group.left ||
			centerX >= group.right ||
			centerY <= band.top ||
			centerY >= band.bottom {
			continue
		}
		column := coordinateInterval(centerX, bandGroup.columns)
		if column < 0 ||
			crossesInternalBoundary(run.run.Bounds, bandGroup.columns) ||
			run.run.Bounds.Top < band.top-tableCoordinateTolerance ||
			run.run.Bounds.Bottom > band.bottom+tableCoordinateTolerance {
			return detectedTable{}, false, nil
		}
		cellRuns[column] = append(cellRuns[column], run)
	}

	cellLines := make([][]textLine, len(cellRuns))
	headerBottom := math.Inf(-1)
	valueTop := math.Inf(1)
	for columnIndex, contained := range cellRuns {
		lines, err := tableCellLines(ctx, contained)
		if err != nil {
			return detectedTable{}, false, err
		}
		if len(lines) != 2 ||
			strings.TrimSpace(lines[0].text) == "" ||
			strings.TrimSpace(lines[1].text) == "" {
			return detectedTable{}, false, nil
		}
		cellLines[columnIndex] = lines
		headerBottom = math.Max(headerBottom, lines[0].bottom)
		valueTop = math.Min(valueTop, lines[1].top)
	}
	if headerBottom >= valueTop-tableCoordinateTolerance ||
		!stackedFieldLinesAligned(cellLines, 0) ||
		!stackedFieldLinesAligned(cellLines, 1) {
		return detectedTable{}, false, nil
	}

	rowBoundary := (headerBottom + valueTop) / 2
	rows := make([][]detectedTableCell, 2)
	for rowIndex := range rows {
		rows[rowIndex] = make([]detectedTableCell, len(cellRuns))
		for columnIndex := range rows[rowIndex] {
			top := band.top
			bottom := rowBoundary
			if rowIndex == 1 {
				top = rowBoundary
				bottom = band.bottom
			}
			rows[rowIndex][columnIndex] = detectedTableCell{
				bounds: document.Rectangle{
					Left:   bandGroup.columns[columnIndex],
					Top:    top,
					Right:  bandGroup.columns[columnIndex+1],
					Bottom: bottom,
				},
				runs: slices.Clone(cellLines[columnIndex][rowIndex].runs),
			}
		}
	}
	return detectedTable{
		bounds: document.Rectangle{
			Left:   group.left,
			Top:    band.top,
			Right:  group.right,
			Bottom: band.bottom,
		},
		columns: bandGroup.columns,
		rows:    rows,
	}, true, nil
}

func stackedFieldLinesAligned(cellLines [][]textLine, lineIndex int) bool {
	minimumTop := math.Inf(1)
	maximumTop := math.Inf(-1)
	minimumBottom := math.Inf(1)
	maximumBottom := math.Inf(-1)
	for _, lines := range cellLines {
		minimumTop = math.Min(minimumTop, lines[lineIndex].top)
		maximumTop = math.Max(maximumTop, lines[lineIndex].top)
		minimumBottom = math.Min(minimumBottom, lines[lineIndex].bottom)
		maximumBottom = math.Max(maximumBottom, lines[lineIndex].bottom)
	}
	return maximumTop-minimumTop <= tableCoordinateTolerance &&
		maximumBottom-minimumBottom <= tableCoordinateTolerance
}

type tableAnchorCluster struct {
	position float64
	rows     map[int]struct{}
}

func inferredHorizontalTable(
	ctx context.Context,
	group horizontalSegmentGroup,
	bandGroup tableBandGroup,
	runs []orderedRun,
) (detectedTable, bool, error) {
	if len(bandGroup.bands) < minimumHorizontalTableBodyRows+1 {
		return detectedTable{}, false, nil
	}
	anchors, err := repeatedTableAnchors(ctx, group, bandGroup, runs)
	if err != nil {
		return detectedTable{}, false, err
	}
	if len(anchors) < 2 ||
		anchors[0]-group.left > maximumFirstAnchorInset {
		return detectedTable{}, false, nil
	}

	columns := []float64{group.left}
	columns = append(columns, anchors[1:]...)
	columns = append(columns, group.right)
	columns = mergeCoordinates(columns)
	if len(columns) < 3 || !validColumnWidths(columns) {
		return detectedTable{}, false, nil
	}

	inferredGroup := bandGroup
	inferredGroup.columns = columns
	table, ok := tableFromBandGroup(group, inferredGroup, true)
	if !ok {
		return detectedTable{}, false, nil
	}
	table.horizontal = true
	return table, true, nil
}

func repeatedTableAnchors(
	ctx context.Context,
	group horizontalSegmentGroup,
	bandGroup tableBandGroup,
	runs []orderedRun,
) ([]float64, error) {
	var clusters []tableAnchorCluster
	for rowIndex, band := range bandGroup.bands {
		starts, err := tableBandWordStarts(ctx, group, band, runs)
		if err != nil {
			return nil, err
		}
		for _, start := range starts {
			clusterIndex := -1
			for index := range clusters {
				if math.Abs(clusters[index].position-start) <= tableCoordinateTolerance {
					clusterIndex = index
					break
				}
			}
			if clusterIndex < 0 {
				clusters = append(clusters, tableAnchorCluster{
					position: start,
					rows:     map[int]struct{}{rowIndex: {}},
				})
				continue
			}
			cluster := &clusters[clusterIndex]
			if _, duplicate := cluster.rows[rowIndex]; duplicate {
				continue
			}
			count := float64(len(cluster.rows))
			cluster.position = (cluster.position*count + start) / (count + 1)
			cluster.rows[rowIndex] = struct{}{}
		}
	}

	requiredBodyRows := requiredHorizontalAnchorBodyRows(
		len(bandGroup.bands) - 1,
	)
	var anchors []float64
	for _, cluster := range clusters {
		if _, inHeader := cluster.rows[0]; !inHeader {
			continue
		}
		bodyRows := 0
		for rowIndex := range cluster.rows {
			if rowIndex > 0 {
				bodyRows++
			}
		}
		if bodyRows >= requiredBodyRows {
			anchors = append(anchors, cluster.position)
		}
	}
	slices.Sort(anchors)
	return mergeCoordinates(anchors), nil
}

func requiredHorizontalAnchorBodyRows(bodyRows int) int {
	if bodyRows <= 2 {
		return minimumHorizontalTableBodyRows
	}
	return max(
		int(math.Ceil(float64(bodyRows)*horizontalAnchorBodySupportRatio)),
		minimumRowsBeforeTableTruncation,
	)
}

func tableBandWordStarts(
	ctx context.Context,
	group horizontalSegmentGroup,
	band tableRowBand,
	runs []orderedRun,
) ([]float64, error) {
	var contained []orderedRun
	for _, run := range runs {
		centerX := (run.run.Bounds.Left + run.run.Bounds.Right) / 2
		centerY := verticalCenter(run.run.Bounds)
		if centerX > group.left &&
			centerX < group.right &&
			centerY > band.top &&
			centerY < band.bottom {
			contained = append(contained, run)
		}
	}
	lines, err := tableCellLines(ctx, contained)
	if err != nil {
		return nil, err
	}

	var starts []float64
	for _, line := range lines {
		atWordStart := true
		var previous *orderedRun
		trackingThreshold := line.trackingGapThreshold()
		for index := range line.runs {
			run := &line.runs[index]
			if strings.TrimSpace(run.text) == "" {
				atWordStart = true
				previous = run
				continue
			}
			if atWordStart ||
				previous != nil && hasWordGap(
					previous.run,
					run.run,
					trackingThreshold,
				) {
				starts = append(starts, run.run.Bounds.Left)
			}
			atWordStart = strings.HasSuffix(run.text, " ")
			previous = run
		}
	}
	return mergeCoordinates(starts), nil
}

func assignHorizontalTableRuns(
	ctx context.Context,
	table *detectedTable,
	runs []orderedRun,
) (bool, error) {
	rowCoordinates := tableRowCoordinates(table)
	invalidRows := make([]bool, len(table.rows))
	for _, run := range runs {
		centerX := (run.run.Bounds.Left + run.run.Bounds.Right) / 2
		centerY := verticalCenter(run.run.Bounds)
		if centerX <= table.bounds.Left ||
			centerX >= table.bounds.Right ||
			centerY <= table.bounds.Top ||
			centerY >= table.bounds.Bottom {
			continue
		}
		row := coordinateInterval(centerY, rowCoordinates)
		column := coordinateInterval(centerX, table.columns)
		if row < 0 {
			continue
		}
		if column < 0 ||
			crossesInternalBoundary(run.run.Bounds, table.columns) ||
			crossesInternalBoundaryY(run.run.Bounds, rowCoordinates) {
			invalidRows[row] = true
			continue
		}
		table.rows[row][column].runs = append(table.rows[row][column].runs, run)
	}
	if invalidRows[0] {
		return false, nil
	}

	end := 1
	for end < len(table.rows) &&
		!invalidRows[end] &&
		horizontalBodyRowComplete(table.rows[end]) {
		end++
	}
	if end < minimumHorizontalTableBodyRows+1 {
		return false, nil
	}
	if end < len(table.rows) &&
		end < minimumRowsBeforeTableTruncation+1 {
		return false, nil
	}
	table.rows = table.rows[:end]
	table.bounds.Bottom = table.rows[len(table.rows)-1][0].bounds.Bottom

	columnHasText := make([]bool, len(table.columns)-1)
	for rowIndex, row := range table.rows {
		for columnIndex, cell := range row {
			hasText := tableCellHasText(cell)
			columnHasText[columnIndex] = columnHasText[columnIndex] || hasText
			if rowIndex == 0 && !hasText {
				return false, nil
			}
		}
	}
	if slices.Contains(columnHasText, false) {
		return false, nil
	}
	return splitHorizontalBodyRows(ctx, table)
}

func horizontalBodyRowComplete(row []detectedTableCell) bool {
	if len(row) < 2 {
		return false
	}
	return tableCellHasText(row[0]) && tableCellHasText(row[len(row)-1])
}

func tableCellHasText(cell detectedTableCell) bool {
	return slices.ContainsFunc(cell.runs, func(run orderedRun) bool {
		return strings.TrimSpace(run.text) != ""
	})
}

type horizontalRowLine struct {
	column int
	line   textLine
}

type horizontalRowAnchor struct {
	position float64
	columns  map[int]struct{}
}

func splitHorizontalBodyRows(
	ctx context.Context,
	table *detectedTable,
) (bool, error) {
	rows := make([][]detectedTableCell, 0, len(table.rows))
	rows = append(rows, table.rows[0])
	for _, row := range table.rows[1:] {
		split, ok, err := splitHorizontalBodyRow(ctx, row)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
		rows = append(rows, split...)
	}
	table.rows = rows
	return true, nil
}

func splitHorizontalBodyRow(
	ctx context.Context,
	row []detectedTableCell,
) ([][]detectedTableCell, bool, error) {
	var lines []horizontalRowLine
	var heights []float64
	cellLines := make([][]textLine, len(row))
	for columnIndex, cell := range row {
		var err error
		cellLines[columnIndex], err = tableCellLines(ctx, cell.runs)
		if err != nil {
			return nil, false, err
		}
		for _, line := range cellLines[columnIndex] {
			lines = append(lines, horizontalRowLine{
				column: columnIndex,
				line:   line,
			})
			heights = append(heights, line.bottom-line.top)
		}
	}
	if len(lines) == 0 {
		return [][]detectedTableCell{row}, true, nil
	}

	slices.Sort(heights)
	continuationDistance := heights[(len(heights)-1)/2]*
		horizontalRowContinuationRatio + tableCoordinateTolerance
	if !hasSeparatedCellLines(cellLines, continuationDistance) {
		return [][]detectedTableCell{row}, true, nil
	}

	anchors := supportedHorizontalRowAnchors(lines)
	anchors = mergeHorizontalRowAnchors(anchors, continuationDistance)
	if len(anchors) < 2 {
		return nil, false, nil
	}
	slices.SortStableFunc(lines, func(left, right horizontalRowLine) int {
		return compareFloat(left.line.top, right.line.top)
	})
	if anchors[0].position-lines[0].line.top > continuationDistance {
		return nil, false, nil
	}

	assignments := make([][]horizontalRowLine, len(anchors))
	for _, item := range lines {
		record := -1
		for index := range anchors {
			if item.line.top >= anchors[index].position-tableCoordinateTolerance {
				record = index
				continue
			}
			break
		}
		if record < 0 {
			if anchors[0].position-item.line.top > continuationDistance {
				return nil, false, nil
			}
			record = 0
		}
		assignments[record] = append(assignments[record], item)
	}

	boundaries := make([]float64, len(anchors)+1)
	boundaries[0] = row[0].bounds.Top
	boundaries[len(boundaries)-1] = row[0].bounds.Bottom
	for index := 1; index < len(anchors); index++ {
		previousBottom := math.Inf(-1)
		currentTop := math.Inf(1)
		for _, item := range assignments[index-1] {
			previousBottom = math.Max(previousBottom, item.line.bottom)
		}
		for _, item := range assignments[index] {
			currentTop = math.Min(currentTop, item.line.top)
		}
		if previousBottom >= currentTop-tableCoordinateTolerance {
			return nil, false, nil
		}
		boundaries[index] = (previousBottom + currentTop) / 2
	}

	result := make([][]detectedTableCell, len(assignments))
	for rowIndex, assignment := range assignments {
		result[rowIndex] = make([]detectedTableCell, len(row))
		populatedColumns := make([]bool, len(row))
		for columnIndex, cell := range row {
			result[rowIndex][columnIndex].bounds = document.Rectangle{
				Left:   cell.bounds.Left,
				Top:    boundaries[rowIndex],
				Right:  cell.bounds.Right,
				Bottom: boundaries[rowIndex+1],
			}
		}
		for _, item := range assignment {
			result[rowIndex][item.column].runs = append(
				result[rowIndex][item.column].runs,
				item.line.runs...,
			)
			populatedColumns[item.column] = true
		}
		if populatedColumnCount(populatedColumns) < 2 {
			return nil, false, nil
		}
	}
	return result, true, nil
}

func hasSeparatedCellLines(lines [][]textLine, continuationDistance float64) bool {
	for _, cell := range lines {
		for index := 1; index < len(cell); index++ {
			if cell[index].top-cell[index-1].bottom > continuationDistance {
				return true
			}
		}
	}
	return false
}

func supportedHorizontalRowAnchors(
	lines []horizontalRowLine,
) []horizontalRowAnchor {
	var anchors []horizontalRowAnchor
	for _, item := range lines {
		anchorIndex := -1
		for index := range anchors {
			if math.Abs(anchors[index].position-item.line.top) <=
				tableCoordinateTolerance {
				anchorIndex = index
				break
			}
		}
		if anchorIndex < 0 {
			anchors = append(anchors, horizontalRowAnchor{
				position: item.line.top,
				columns:  map[int]struct{}{item.column: {}},
			})
			continue
		}
		anchor := &anchors[anchorIndex]
		if _, duplicate := anchor.columns[item.column]; duplicate {
			continue
		}
		count := float64(len(anchor.columns))
		anchor.position = (anchor.position*count + item.line.top) / (count + 1)
		anchor.columns[item.column] = struct{}{}
	}
	anchors = slices.DeleteFunc(anchors, func(anchor horizontalRowAnchor) bool {
		return len(anchor.columns) < 2
	})
	slices.SortStableFunc(anchors, func(left, right horizontalRowAnchor) int {
		return compareFloat(left.position, right.position)
	})
	return anchors
}

func mergeHorizontalRowAnchors(
	anchors []horizontalRowAnchor,
	continuationDistance float64,
) []horizontalRowAnchor {
	merged := anchors[:0]
	for _, anchor := range anchors {
		if len(merged) == 0 ||
			anchor.position-merged[len(merged)-1].position >
				continuationDistance {
			merged = append(merged, anchor)
			continue
		}
		current := &merged[len(merged)-1]
		for column := range anchor.columns {
			current.columns[column] = struct{}{}
		}
	}
	return merged
}

func populatedColumnCount(columns []bool) int {
	count := 0
	for _, populated := range columns {
		if populated {
			count++
		}
	}
	return count
}

func mergeCoordinates(coordinates []float64) []float64 {
	slices.Sort(coordinates)
	merged := coordinates[:0]
	for _, coordinate := range coordinates {
		if len(merged) == 0 ||
			coordinate-merged[len(merged)-1] > tableCoordinateTolerance {
			merged = append(merged, coordinate)
			continue
		}
		merged[len(merged)-1] = (merged[len(merged)-1] + coordinate) / 2
	}
	return merged
}

func validColumnWidths(columns []float64) bool {
	for index := 1; index < len(columns); index++ {
		if columns[index]-columns[index-1] < minimumTableCellWidth {
			return false
		}
	}
	return true
}

func sameCoordinates(left, right []float64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if math.Abs(left[index]-right[index]) > tableCoordinateTolerance {
			return false
		}
	}
	return true
}

func assignTableRuns(table *detectedTable, runs []orderedRun) bool {
	columnHasText := make([]bool, len(table.columns)-1)
	headerHasText := make([]bool, len(table.columns)-1)
	for _, run := range runs {
		centerX := (run.run.Bounds.Left + run.run.Bounds.Right) / 2
		centerY := verticalCenter(run.run.Bounds)
		if centerX <= table.bounds.Left ||
			centerX >= table.bounds.Right ||
			centerY <= table.bounds.Top ||
			centerY >= table.bounds.Bottom {
			continue
		}
		row := coordinateInterval(centerY, tableRowCoordinates(table))
		column := coordinateInterval(centerX, table.columns)
		if row < 0 || column < 0 ||
			crossesInternalBoundary(run.run.Bounds, table.columns) ||
			crossesInternalBoundaryY(run.run.Bounds, tableRowCoordinates(table)) {
			return false
		}
		table.rows[row][column].runs = append(table.rows[row][column].runs, run)
		if strings.TrimSpace(run.text) != "" {
			columnHasText[column] = true
			if row == 0 {
				headerHasText[column] = true
			}
		}
	}
	return !slices.Contains(columnHasText, false) &&
		!slices.Contains(headerHasText, false)
}

func hasDistinctTableHeader(table detectedTable) bool {
	var headerWeights []int
	var bodyWeights []int
	for rowIndex, row := range table.rows {
		for _, cell := range row {
			for _, run := range cell.runs {
				if strings.TrimSpace(run.text) == "" {
					continue
				}
				if rowIndex == 0 {
					headerWeights = append(headerWeights, run.run.Style.FontWeight)
				} else {
					bodyWeights = append(bodyWeights, run.run.Style.FontWeight)
				}
			}
		}
	}
	if len(headerWeights) == 0 {
		return false
	}
	slices.Sort(headerWeights)
	headerWeight := headerWeights[(len(headerWeights)-1)/2]
	if len(bodyWeights) == 0 {
		return headerWeight >= 600
	}
	slices.Sort(bodyWeights)
	bodyWeight := bodyWeights[(len(bodyWeights)-1)/2]
	return headerWeight >= 600 && headerWeight >= bodyWeight+100
}

func hasReliableTableHeader(table detectedTable) bool {
	if hasDistinctTableHeader(table) {
		return true
	}
	if table.horizontal {
		return false
	}
	bodyHasText := false
	headerOnlyColumn := false
	for columnIndex := range table.rows[0] {
		columnHasBodyText := false
		for rowIndex := 1; rowIndex < len(table.rows); rowIndex++ {
			if tableCellHasText(table.rows[rowIndex][columnIndex]) {
				columnHasBodyText = true
				bodyHasText = true
				break
			}
		}
		headerOnlyColumn = headerOnlyColumn || !columnHasBodyText
	}
	return bodyHasText && headerOnlyColumn
}

func tableRowCoordinates(table *detectedTable) []float64 {
	coordinates := make([]float64, len(table.rows)+1)
	coordinates[0] = table.bounds.Top
	for index := range table.rows {
		coordinates[index+1] = table.rows[index][0].bounds.Bottom
	}
	return coordinates
}

func coordinateInterval(value float64, boundaries []float64) int {
	for index := 1; index < len(boundaries); index++ {
		if value > boundaries[index-1] && value < boundaries[index] {
			return index - 1
		}
	}
	return -1
}

func crossesInternalBoundary(
	bounds document.Rectangle,
	boundaries []float64,
) bool {
	for _, boundary := range boundaries[1 : len(boundaries)-1] {
		if bounds.Left < boundary-tableCoordinateTolerance &&
			bounds.Right > boundary+tableCoordinateTolerance {
			return true
		}
	}
	return false
}

func crossesInternalBoundaryY(
	bounds document.Rectangle,
	boundaries []float64,
) bool {
	for _, boundary := range boundaries[1 : len(boundaries)-1] {
		if bounds.Top < boundary-tableCoordinateTolerance &&
			bounds.Bottom > boundary+tableCoordinateTolerance {
			return true
		}
	}
	return false
}

func orderedTableContent(
	ctx context.Context,
	table detectedTable,
) ([][]tableCellContent, error) {
	rows := make([][]tableCellContent, len(table.rows))
	for rowIndex, row := range table.rows {
		rows[rowIndex] = make([]tableCellContent, len(row))
		for columnIndex, cell := range row {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			lines, err := tableCellLines(ctx, cell.runs)
			if err != nil {
				return nil, err
			}
			text, links := joinWrappedContent(lines)
			rows[rowIndex][columnIndex] = tableCellContent{
				text:  text,
				links: links,
			}
		}
	}
	return rows, nil
}

func semanticTable(
	ctx context.Context,
	table detectedTable,
) (*document.Table, error) {
	content, err := orderedTableContent(ctx, table)
	if err != nil {
		return nil, err
	}
	result := &document.Table{Rows: make([]document.TableRow, len(content))}
	for rowIndex, row := range content {
		result.Rows[rowIndex].Cells = make([]document.TableCell, len(row))
		for columnIndex, cell := range row {
			result.Rows[rowIndex].Cells[columnIndex] = document.TableCell{
				Text:  cell.text,
				Links: cell.links,
			}
		}
	}
	return result, nil
}

func lineTableMembership(line textLine, table detectedTable) (bool, bool) {
	if len(line.runs) == 0 {
		return false, false
	}
	inside := 0
	for _, run := range line.runs {
		centerX := (run.run.Bounds.Left + run.run.Bounds.Right) / 2
		centerY := verticalCenter(run.run.Bounds)
		if centerX > table.bounds.Left &&
			centerX < table.bounds.Right &&
			centerY > table.bounds.Top &&
			centerY < table.bounds.Bottom {
			inside++
		}
	}
	return inside > 0, inside == len(line.runs)
}

func tableCellLines(
	ctx context.Context,
	runs []orderedRun,
) ([]textLine, error) {
	runs = slices.Clone(runs)
	slices.SortStableFunc(runs, compareRunsVertically)
	var lines []textLine
	for _, run := range runs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(lines) == 0 || !belongsToLine(lines[len(lines)-1], run.run) {
			lines = append(lines, newTextLine(run))
			continue
		}
		lines[len(lines)-1].add(run)
	}
	for index := range lines {
		if err := lines[index].finish(ctx); err != nil {
			return nil, err
		}
	}
	slices.SortStableFunc(lines, compareLines)
	return lines, nil
}
