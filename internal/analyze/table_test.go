package analyze

import (
	"context"
	"reflect"
	"testing"

	"github.com/Patrick-Q-Jensen/TransmuteMD/internal/document"
)

func TestDetectTablesAssignsRectangularCells(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 110, 10),
		horizontalRuling(10, 110, 30),
		horizontalRuling(10, 110, 50),
		verticalRuling(60, 10, 50),
	}
	runs := []orderedRun{
		tableRun("Left", 15, 15, 35, 25),
		tableRun("Right", 65, 15, 95, 25),
		tableRun("A", 15, 35, 25, 45),
		tableRun("B", 65, 35, 75, 45),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  100,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 1 {
		t.Fatalf("table count = %d, want 1", len(result.tables))
	}
	table := result.tables[0]
	if len(table.rows) != 2 || len(table.columns) != 3 {
		t.Fatalf("table shape = %dx%d, want 2x2", len(table.rows), len(table.columns)-1)
	}
	if got := table.rows[0][0].runs[0].text; got != "Left" {
		t.Fatalf("header cell text = %q, want Left", got)
	}
	if got := table.rows[1][1].runs[0].text; got != "B" {
		t.Fatalf("body cell text = %q, want B", got)
	}
}

func TestDetectTablesSegmentsStableGridAfterFullWidthRow(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 110, 10),
		horizontalRuling(10, 110, 30),
		horizontalRuling(10, 110, 50),
		horizontalRuling(10, 110, 70),
		verticalRuling(60, 30, 70),
	}
	runs := []orderedRun{
		styledTableRun("Section", 15, 15, 50, 25, 700),
		styledTableRun("Name", 15, 35, 35, 45, 700),
		styledTableRun("Result", 65, 35, 95, 45, 700),
		styledTableRun("Case A", 15, 55, 40, 65, 400),
		styledTableRun("Passed", 65, 55, 95, 65, 400),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  100,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 1 {
		t.Fatalf("table count = %d, want 1", len(result.tables))
	}
	table := result.tables[0]
	if table.bounds.Top != 30 || table.bounds.Bottom != 70 {
		t.Fatalf("table bounds = %+v, want top 30 and bottom 70", table.bounds)
	}
	if len(result.rejected) != 0 {
		t.Fatalf("rejected regions = %#v, want none", result.rejected)
	}
}

func TestOrderedTableContentOrdersWrappedCellsByRowAndColumn(t *testing.T) {
	t.Parallel()

	table := detectedTable{
		rows: [][]detectedTableCell{
			{
				{runs: []orderedRun{tableRun("Description", 10, 10, 70, 20)}},
				{runs: []orderedRun{tableRun("Result", 80, 10, 110, 20)}},
			},
			{
				{
					runs: []orderedRun{
						tableRun("uration", 10, 35, 45, 45),
						tableRun("Config-", 10, 22, 45, 32),
					},
				},
				{runs: []orderedRun{tableRun("Passed", 80, 22, 110, 32)}},
			},
		},
	}

	got, err := orderedTableContent(context.Background(), table)
	if err != nil {
		t.Fatalf("orderedTableContent() returned an unexpected error: %v", err)
	}
	want := [][]tableCellContent{
		{{text: "Description"}, {text: "Result"}},
		{{text: "Configuration"}, {text: "Passed"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("table content = %#v, want %#v", got, want)
	}
}

func TestDetectTablesRejectsMergedCellGrid(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 110, 10),
		horizontalRuling(10, 110, 30),
		horizontalRuling(10, 110, 50),
		verticalRuling(60, 30, 50),
	}
	runs := []orderedRun{
		tableRun("Merged header", 15, 15, 95, 25),
		tableRun("A", 15, 35, 25, 45),
		tableRun("B", 65, 35, 75, 45),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  100,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 0 {
		t.Fatalf("table count = %d, want 0", len(result.tables))
	}
}

func TestDetectTablesReportsMergedRuledRegionAsFallback(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 110, 10),
		horizontalRuling(10, 110, 30),
		horizontalRuling(10, 110, 50),
		horizontalRuling(10, 110, 70),
		verticalRuling(60, 30, 70),
	}
	runs := []orderedRun{
		tableRun("Merged header", 15, 15, 95, 25),
		tableRun("A", 15, 35, 25, 45),
		tableRun("B", 65, 35, 75, 45),
		tableRun("C", 15, 55, 25, 65),
		tableRun("D", 65, 55, 75, 65),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  100,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 0 {
		t.Fatalf("table count = %d, want 0", len(result.tables))
	}
	if len(result.rejected) != 1 {
		t.Fatalf("rejected region count = %d, want 1", len(result.rejected))
	}
}

func TestDetectTablesRejectsTextCrossingCellBoundary(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 110, 10),
		horizontalRuling(10, 110, 30),
		horizontalRuling(10, 110, 50),
		verticalRuling(60, 10, 50),
	}
	runs := []orderedRun{
		tableRun("Left", 15, 15, 35, 25),
		tableRun("Right", 65, 15, 95, 25),
		tableRun("crossing", 50, 35, 70, 45),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  100,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 0 {
		t.Fatalf("table count = %d, want 0", len(result.tables))
	}
	if len(result.rejected) != 1 {
		t.Fatalf("rejected region count = %d, want 1", len(result.rejected))
	}
}

func TestDetectTablesHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := detectTables(ctx, document.Page{
		Height: 100,
		Rulings: []document.Ruling{
			horizontalRuling(10, 110, 10),
		},
	}, nil)
	if err == nil {
		t.Fatal("detectTables() returned nil error for cancelled context")
	}
}

func horizontalRuling(left, right, y float64) document.Ruling {
	return document.Ruling{
		Start: document.Point{X: left, Y: y},
		End:   document.Point{X: right, Y: y},
		Width: 1,
	}
}

func verticalRuling(x, top, bottom float64) document.Ruling {
	return document.Ruling{
		Start: document.Point{X: x, Y: top},
		End:   document.Point{X: x, Y: bottom},
		Width: 1,
	}
}

func tableRun(text string, left, top, right, bottom float64) orderedRun {
	return styledTableRun(text, left, top, right, bottom, 0)
}

func styledTableRun(
	text string,
	left,
	top,
	right,
	bottom float64,
	weight int,
) orderedRun {
	return orderedRun{
		run: document.TextRun{
			Text: text,
			Bounds: document.Rectangle{
				Left:   left,
				Top:    top,
				Right:  right,
				Bottom: bottom,
			},
			Style: document.TextStyle{FontWeight: weight},
		},
		text: text,
	}
}
