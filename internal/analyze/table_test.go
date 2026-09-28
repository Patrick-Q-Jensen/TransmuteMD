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

func TestDetectTablesAcceptsEmptyBodyRowsInCompleteGrid(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 110, 10),
		horizontalRuling(10, 110, 30),
		horizontalRuling(10, 110, 50),
		verticalRuling(60, 10, 50),
	}
	runs := []orderedRun{
		styledTableRun("Name", 15, 15, 35, 25, 700),
		styledTableRun("Result", 65, 15, 95, 25, 700),
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
	if got := len(result.tables[0].rows); got != 2 {
		t.Fatalf("row count = %d, want 2", got)
	}
	if tableCellHasText(result.tables[0].rows[1][0]) ||
		tableCellHasText(result.tables[0].rows[1][1]) {
		t.Fatal("empty body row unexpectedly contains text")
	}
}

func TestDetectTablesAcceptsEmptySegmentWithBoldHeader(t *testing.T) {
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

func TestDetectTablesUsesCompleteGridWhenSegmentHeaderIsRegularWeight(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 110, 10),
		horizontalRuling(10, 110, 30),
		horizontalRuling(10, 110, 50),
		horizontalRuling(10, 110, 70),
		horizontalRuling(10, 110, 90),
		verticalRuling(40, 30, 90),
	}
	runs := []orderedRun{
		styledTableRun("Section", 15, 15, 50, 25, 700),
		styledTableRun("Choice", 15, 35, 35, 45, 400),
		styledTableRun("Result", 45, 35, 75, 45, 400),
		styledTableRun("Passed", 45, 55, 75, 65, 400),
		styledTableRun("Failed", 45, 75, 75, 85, 400),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  120,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 1 {
		t.Fatalf("table count = %d, want 1", len(result.tables))
	}
	table := result.tables[0]
	if table.bounds.Top != 30 || table.bounds.Bottom != 90 {
		t.Fatalf("table bounds = %+v, want top 30 and bottom 90", table.bounds)
	}
	if tableCellHasText(table.rows[1][0]) || tableCellHasText(table.rows[2][0]) {
		t.Fatal("blank choice cells unexpectedly contain text")
	}
}

func TestDetectTablesInfersRepeatedColumnsWithinHorizontalRules(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 110, 10),
		horizontalRuling(10, 110, 30),
		horizontalRuling(10, 110, 50),
		horizontalRuling(10, 110, 70),
		horizontalRuling(10, 110, 90),
		horizontalRuling(10, 110, 110),
	}
	runs := []orderedRun{
		styledTableRun("Name", 12, 15, 32, 25, 700),
		styledTableRun("Result", 65, 15, 95, 25, 700),
		styledTableRun("A", 12, 35, 20, 45, 400),
		styledTableRun("Passed", 65, 35, 95, 45, 400),
		styledTableRun("B", 12, 55, 20, 65, 400),
		styledTableRun("Failed", 65, 55, 95, 65, 400),
		styledTableRun("C", 12, 75, 20, 85, 400),
		styledTableRun("Passed", 65, 75, 95, 85, 400),
		styledTableRun("Next section", 12, 95, 55, 105, 700),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  140,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 1 {
		t.Fatalf("table count = %d, want 1", len(result.tables))
	}
	table := result.tables[0]
	if len(table.rows) != 4 || len(table.columns) != 3 {
		t.Fatalf("table shape = %dx%d, want 4x2", len(table.rows), len(table.columns)-1)
	}
	if table.bounds.Bottom != 90 {
		t.Fatalf("table bottom = %v, want 90 before following section", table.bounds.Bottom)
	}
}

func TestDetectTablesInfersColumnsFromOneBodyRow(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 110, 10),
		horizontalRuling(10, 110, 30),
		horizontalRuling(10, 110, 50),
	}
	runs := []orderedRun{
		styledTableRun("Term", 12, 15, 32, 25, 700),
		styledTableRun("Definition", 65, 15, 100, 25, 700),
		styledTableRun("CLI", 12, 35, 25, 45, 400),
		styledTableRun("Command line", 65, 35, 105, 45, 400),
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
}

func TestDetectTablesSegmentsAdjacentHorizontalSchemas(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 130, 10),
		horizontalRuling(10, 130, 30),
		horizontalRuling(10, 130, 50),
		horizontalRuling(10, 130, 70),
		horizontalRuling(10, 130, 90),
		horizontalRuling(10, 130, 110),
	}
	runs := []orderedRun{
		styledTableRun("Key", 12, 15, 25, 25, 700),
		styledTableRun("Value", 50, 15, 75, 25, 700),
		styledTableRun("A", 12, 35, 20, 45, 400),
		styledTableRun("One", 50, 35, 70, 45, 400),
		styledTableRun("Next table", 12, 55, 55, 65, 700),
		styledTableRun("ID", 12, 75, 22, 85, 700),
		styledTableRun("Description", 40, 75, 80, 85, 700),
		styledTableRun("State", 100, 75, 120, 85, 700),
		styledTableRun("1", 12, 95, 18, 105, 400),
		styledTableRun("Item", 40, 95, 60, 105, 400),
		styledTableRun("Done", 100, 95, 120, 105, 400),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  140,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 2 {
		t.Fatalf("table count = %d, want 2", len(result.tables))
	}
	if got := len(result.tables[0].columns) - 1; got != 2 {
		t.Fatalf("first table column count = %d, want 2", got)
	}
	if got := len(result.tables[1].columns) - 1; got != 3 {
		t.Fatalf("second table column count = %d, want 3", got)
	}
	if len(result.rejected) != 0 {
		t.Fatalf("rejected regions = %#v, want none", result.rejected)
	}
}

func TestDetectTablesKeepsAdjacentShortSchemasAndLinksIndependent(t *testing.T) {
	t.Parallel()

	target := document.LinkTarget{
		Kind: document.LinkTargetExternal,
		URI:  "https://example.com/reference",
	}
	rulings := []document.Ruling{
		horizontalRuling(10, 130, 10),
		horizontalRuling(10, 130, 30),
		horizontalRuling(10, 130, 50),
		horizontalRuling(10, 130, 70),
		horizontalRuling(10, 130, 90),
	}
	runs := []orderedRun{
		styledTableRun("Key", 12, 15, 25, 25, 700),
		styledTableRun("Identity", 45, 15, 75, 25, 700),
		styledTableRun("Title", 95, 15, 120, 25, 700),
		styledTableRun("A", 12, 35, 20, 45, 400),
		styledTableRun("One", 45, 35, 65, 45, 400),
		linkedStyledTableRun("Linked", 95, 35, 120, 45, 400, target),
		styledTableRun("Name", 12, 55, 32, 65, 700),
		styledTableRun("Result", 65, 55, 95, 65, 700),
		styledTableRun("Case", 12, 75, 32, 85, 400),
		styledTableRun("Passed", 65, 75, 95, 85, 400),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  120,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 2 {
		t.Fatalf("table count = %d, want 2", len(result.tables))
	}
	if got := len(result.tables[0].columns) - 1; got != 3 {
		t.Fatalf("first table column count = %d, want 3", got)
	}
	if got := len(result.tables[1].columns) - 1; got != 2 {
		t.Fatalf("second table column count = %d, want 2", got)
	}
	first, err := semanticTable(context.Background(), result.tables[0])
	if err != nil {
		t.Fatalf("semanticTable() returned an unexpected error: %v", err)
	}
	gotLinks := first.Rows[1].Cells[2].Links
	wantLinks := []document.TextLink{{
		Start:  0,
		End:    len("Linked"),
		Target: target,
	}}
	if !reflect.DeepEqual(gotLinks, wantLinks) {
		t.Fatalf("linked cell links = %#v, want %#v", gotLinks, wantLinks)
	}
}

func TestDetectTablesSplitsAlignedRecordsWithinTallHorizontalBand(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 130, 10),
		horizontalRuling(10, 130, 30),
		horizontalRuling(10, 130, 140),
	}
	runs := []orderedRun{
		styledTableRun("Group", 12, 15, 32, 25, 700),
		styledTableRun("Change", 40, 15, 70, 25, 700),
		styledTableRun("Audit", 90, 15, 115, 25, 700),
		styledTableRun("A", 12, 35, 20, 45, 400),
		styledTableRun("Initial", 40, 35, 65, 45, 400),
		styledTableRun("First update", 40, 65, 75, 75, 400),
		styledTableRun("2026-01-01", 90, 65, 125, 75, 400),
		styledTableRun("Owner A", 90, 78, 120, 88, 400),
		styledTableRun("Second update", 40, 105, 80, 115, 400),
		styledTableRun("2026-02-01", 90, 105, 125, 115, 400),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  170,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 1 {
		t.Fatalf("table count = %d, want 1", len(result.tables))
	}
	content, err := orderedTableContent(context.Background(), result.tables[0])
	if err != nil {
		t.Fatalf("orderedTableContent() returned an unexpected error: %v", err)
	}
	want := [][]tableCellContent{
		{{text: "Group"}, {text: "Change"}, {text: "Audit"}},
		{{text: "A"}, {text: "Initial"}, {}},
		{{}, {text: "First update"}, {text: "2026-01-01 Owner A"}},
		{{}, {text: "Second update"}, {text: "2026-02-01"}},
	}
	if !reflect.DeepEqual(content, want) {
		t.Fatalf("table content = %#v, want %#v", content, want)
	}
}

func TestDetectTablesKeepsStackedHeaderInOneSemanticRow(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 130, 10),
		horizontalRuling(10, 130, 40),
		horizontalRuling(10, 130, 70),
	}
	runs := []orderedRun{
		styledTableRun("ID", 12, 15, 22, 25, 700),
		styledTableRun("Primary", 40, 15, 70, 25, 700),
		styledTableRun("Secondary", 40, 27, 75, 37, 700),
		styledTableRun("Date", 90, 15, 110, 25, 700),
		styledTableRun("Owner", 90, 27, 115, 37, 700),
		styledTableRun("A", 12, 45, 20, 55, 400),
		styledTableRun("Updated", 40, 45, 70, 55, 400),
		styledTableRun("Today", 90, 45, 115, 55, 400),
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
	content, err := orderedTableContent(context.Background(), result.tables[0])
	if err != nil {
		t.Fatalf("orderedTableContent() returned an unexpected error: %v", err)
	}
	if len(content) != 2 {
		t.Fatalf("semantic row count = %d, want 2", len(content))
	}
	if got, want := content[0][1].text, "Primary Secondary"; got != want {
		t.Fatalf("stacked header text = %q, want %q", got, want)
	}
	if got, want := content[0][2].text, "Date Owner"; got != want {
		t.Fatalf("stacked header text = %q, want %q", got, want)
	}
}

func TestDetectTablesKeepsCloseWrappedBodyLinesInOneRow(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 130, 10),
		horizontalRuling(10, 130, 30),
		horizontalRuling(10, 130, 70),
	}
	runs := []orderedRun{
		styledTableRun("ID", 12, 15, 22, 25, 700),
		styledTableRun("Description", 40, 15, 75, 25, 700),
		styledTableRun("Result", 90, 15, 115, 25, 700),
		styledTableRun("A", 12, 35, 20, 45, 400),
		styledTableRun("Long", 40, 35, 60, 45, 400),
		styledTableRun("Passed", 90, 35, 115, 45, 400),
		styledTableRun("value", 40, 47, 60, 57, 400),
		styledTableRun("today", 90, 47, 112, 57, 400),
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
	content, err := orderedTableContent(context.Background(), result.tables[0])
	if err != nil {
		t.Fatalf("orderedTableContent() returned an unexpected error: %v", err)
	}
	if len(content) != 2 {
		t.Fatalf("semantic row count = %d, want 2", len(content))
	}
	if got, want := content[1][1].text, "Long value"; got != want {
		t.Fatalf("wrapped body text = %q, want %q", got, want)
	}
	if got, want := content[1][2].text, "Passed today"; got != want {
		t.Fatalf("wrapped body text = %q, want %q", got, want)
	}
}

func TestDetectTablesRejectsUnsynchronizedRecordsWithinTallBand(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 130, 10),
		horizontalRuling(10, 130, 30),
		horizontalRuling(10, 130, 140),
	}
	runs := []orderedRun{
		styledTableRun("Group", 12, 15, 32, 25, 700),
		styledTableRun("Change", 40, 15, 70, 25, 700),
		styledTableRun("Audit", 90, 15, 115, 25, 700),
		styledTableRun("A", 12, 35, 20, 45, 400),
		styledTableRun("First", 40, 35, 60, 45, 400),
		styledTableRun("2026-01-01", 90, 55, 125, 65, 400),
		styledTableRun("Second", 40, 80, 65, 90, 400),
		styledTableRun("2026-02-01", 90, 100, 125, 110, 400),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  170,
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

func TestDetectTablesInfersThreeColumnsAndKeepsWrappedCellText(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 130, 10),
		horizontalRuling(10, 130, 30),
		horizontalRuling(10, 130, 55),
		horizontalRuling(10, 130, 80),
		horizontalRuling(10, 130, 105),
	}
	runs := []orderedRun{
		styledTableRun("ID", 12, 15, 22, 25, 700),
		styledTableRun("Description", 35, 15, 75, 25, 700),
		styledTableRun("Case", 100, 15, 120, 25, 700),
		styledTableRun("1", 12, 35, 18, 45, 400),
		styledTableRun("Wrapped", 35, 35, 65, 45, 400),
		styledTableRun("TC-1", 100, 35, 120, 45, 400),
		styledTableRun("description", 35, 45, 75, 52, 400),
		styledTableRun("2", 12, 60, 18, 70, 400),
		styledTableRun("Second", 35, 60, 60, 70, 400),
		styledTableRun("TC-2", 100, 60, 120, 70, 400),
		styledTableRun("3", 12, 85, 18, 95, 400),
		styledTableRun("Third", 35, 85, 55, 95, 400),
		styledTableRun("TC-3", 100, 85, 120, 95, 400),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  140,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 1 {
		t.Fatalf("table count = %d, want 1", len(result.tables))
	}
	table := result.tables[0]
	if len(table.rows) != 4 || len(table.columns) != 4 {
		t.Fatalf("table shape = %dx%d, want 4x3", len(table.rows), len(table.columns)-1)
	}
	content, err := orderedTableContent(context.Background(), table)
	if err != nil {
		t.Fatalf("orderedTableContent() returned an unexpected error: %v", err)
	}
	if got, want := content[1][1].text, "Wrapped description"; got != want {
		t.Fatalf("wrapped cell text = %q, want %q", got, want)
	}
}

func TestDetectTablesInfersSparseInternalColumnInShortTable(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 130, 10),
		horizontalRuling(10, 130, 30),
		horizontalRuling(10, 130, 55),
		horizontalRuling(10, 130, 80),
	}
	runs := []orderedRun{
		styledTableRun("ID", 12, 15, 22, 25, 700),
		styledTableRun("Scope", 40, 15, 65, 25, 700),
		styledTableRun("Result", 95, 15, 120, 25, 700),
		styledTableRun("A", 12, 35, 18, 45, 400),
		styledTableRun("All", 40, 35, 52, 45, 400),
		styledTableRun("Passed", 95, 35, 120, 45, 400),
		styledTableRun("B", 12, 60, 18, 70, 400),
		styledTableRun("Pending", 95, 60, 120, 70, 400),
	}

	result, err := detectTables(context.Background(), document.Page{
		Height:  110,
		Rulings: rulings,
	}, runs)
	if err != nil {
		t.Fatalf("detectTables() returned an unexpected error: %v", err)
	}
	if len(result.tables) != 1 {
		t.Fatalf("table count = %d, want 1", len(result.tables))
	}
	table := result.tables[0]
	if len(table.rows) != 3 || len(table.columns) != 4 {
		t.Fatalf("table shape = %dx%d, want 3x3", len(table.rows), len(table.columns)-1)
	}
	if tableCellHasText(table.rows[2][1]) {
		t.Fatal("sparse internal body cell unexpectedly contains text")
	}
}

func TestDetectTablesRejectsWeakHorizontalAnchorEvidence(t *testing.T) {
	t.Parallel()

	rulings := []document.Ruling{
		horizontalRuling(10, 110, 10),
		horizontalRuling(10, 110, 30),
		horizontalRuling(10, 110, 50),
		horizontalRuling(10, 110, 70),
	}
	runs := []orderedRun{
		styledTableRun("Name", 12, 15, 32, 25, 700),
		styledTableRun("Result", 65, 15, 95, 25, 700),
		styledTableRun("A", 12, 35, 20, 45, 400),
		styledTableRun("Passed", 65, 35, 95, 45, 400),
		styledTableRun("B", 12, 55, 20, 65, 400),
		styledTableRun("Failed", 45, 55, 75, 65, 400),
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

func linkedStyledTableRun(
	text string,
	left,
	top,
	right,
	bottom float64,
	weight int,
	target document.LinkTarget,
) orderedRun {
	run := styledTableRun(text, left, top, right, bottom, weight)
	run.linkTarget = target
	return run
}
