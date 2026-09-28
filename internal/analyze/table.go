package analyze

import (
	"context"
	"math"
	"slices"
	"strings"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

const (
	tableCoordinateTolerance = 2.5
	minimumTableCellHeight   = 4.0
	minimumTableCellWidth    = 8.0
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
			if !ok {
				tableLike, err := hasTableCandidateText(ctx, bandBounds, runs)
				if err != nil {
					return tableDetection{}, err
				}
				if tableLike {
					result.rejected = append(result.rejected, bandBounds)
				}
				continue
			}
			if !assignTableRuns(&table, runs) ||
				table.requireHeaderStyle && !hasDistinctTableHeader(table) {
				result.rejected = append(result.rejected, table.bounds)
				continue
			}
			result.tables = append(result.tables, table)
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
	if len(group.lines) < 3 {
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
	hasBodyText := false
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
			} else {
				hasBodyText = true
			}
		}
	}
	return hasBodyText &&
		!slices.Contains(columnHasText, false) &&
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
	if len(headerWeights) == 0 || len(bodyWeights) == 0 {
		return false
	}
	slices.Sort(headerWeights)
	slices.Sort(bodyWeights)
	headerWeight := headerWeights[(len(headerWeights)-1)/2]
	bodyWeight := bodyWeights[(len(bodyWeights)-1)/2]
	return headerWeight >= 600 && headerWeight >= bodyWeight+100
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
