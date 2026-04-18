package engine

import (
	"fmt"
	"strings"
)

// ColumnDiff represents a change to a column between two schemas.
type ColumnDiff struct {
	Name     string
	Status   string // "added", "removed", "modified"
	OldType  string // only for "modified"
	NewType  string // only for "modified"
	OldExtra string
	NewExtra string
}

// TableDiff represents the diff for a single table.
type TableDiff struct {
	Name       string
	Status     string // "added", "removed", "modified"
	ColumnDiffs []ColumnDiff
}

// SchemaDiffResult holds the complete diff between two schemas.
type SchemaDiffResult struct {
	Tables []TableDiff
}

// ComputeSchemaDiff compares source and target table/column metadata.
func ComputeSchemaDiff(source, target map[string][]ColumnInfo) SchemaDiffResult {
	var result SchemaDiffResult

	// Tables in target but not in source → added
	for name, targetCols := range target {
		if _, exists := source[name]; !exists {
			td := TableDiff{Name: name, Status: "added"}
			for _, col := range targetCols {
				td.ColumnDiffs = append(td.ColumnDiffs, ColumnDiff{
					Name:    col.Name,
					Status:  "added",
					NewType: col.DataType,
				})
			}
			result.Tables = append(result.Tables, td)
		}
	}

	// Tables in source but not in target → removed
	for name, sourceCols := range source {
		if _, exists := target[name]; !exists {
			td := TableDiff{Name: name, Status: "removed"}
			for _, col := range sourceCols {
				td.ColumnDiffs = append(td.ColumnDiffs, ColumnDiff{
					Name:    col.Name,
					Status:  "removed",
					OldType: col.DataType,
				})
			}
			result.Tables = append(result.Tables, td)
		}
	}

	// Tables in both → compare columns
	for name, sourceCols := range source {
		targetCols, exists := target[name]
		if !exists {
			continue
		}

		td := TableDiff{Name: name}

		sourceMap := make(map[string]ColumnInfo)
		for _, col := range sourceCols {
			sourceMap[col.Name] = col
		}
		targetMap := make(map[string]ColumnInfo)
		for _, col := range targetCols {
			targetMap[col.Name] = col
		}

		// Added columns
		for colName, col := range targetMap {
			if _, exists := sourceMap[colName]; !exists {
				td.ColumnDiffs = append(td.ColumnDiffs, ColumnDiff{
					Name:    colName,
					Status:  "added",
					NewType: col.DataType,
				})
			}
		}

		// Removed columns
		for colName, col := range sourceMap {
			if _, exists := targetMap[colName]; !exists {
				td.ColumnDiffs = append(td.ColumnDiffs, ColumnDiff{
					Name:    colName,
					Status:  "removed",
					OldType: col.DataType,
				})
			}
		}

		// Modified columns
		for colName, sourceCol := range sourceMap {
			targetCol, exists := targetMap[colName]
			if !exists {
				continue
			}
			if sourceCol.DataType != targetCol.DataType ||
				sourceCol.Nullable != targetCol.Nullable ||
				sourceCol.IsPrimaryKey != targetCol.IsPrimaryKey ||
				sourceCol.Default != targetCol.Default {
				td.ColumnDiffs = append(td.ColumnDiffs, ColumnDiff{
					Name:     colName,
					Status:   "modified",
					OldType:  formatColumnDetail(sourceCol),
					NewType:  formatColumnDetail(targetCol),
					OldExtra: sourceCol.Extra,
					NewExtra: targetCol.Extra,
				})
			}
		}

		if len(td.ColumnDiffs) > 0 {
			td.Status = "modified"
			result.Tables = append(result.Tables, td)
		}
	}

	return result
}

func formatColumnDetail(col ColumnInfo) string {
	var parts []string
	parts = append(parts, col.DataType)
	if col.IsPrimaryKey {
		parts = append(parts, "PRIMARY KEY")
	}
	if !col.Nullable {
		parts = append(parts, "NOT NULL")
	}
	if col.Default != "" {
		parts = append(parts, fmt.Sprintf("DEFAULT %s", col.Default))
	}
	return strings.Join(parts, " ")
}

// GenerateColumnDiffText generates unified diff text for a table's column changes.
func GenerateColumnDiffText(td TableDiff) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("--- source/%s", td.Name))
	lines = append(lines, fmt.Sprintf("+++ target/%s", td.Name))

	// Count additions and removals for the hunk header
	oldCount, newCount := 0, 0
	for _, cd := range td.ColumnDiffs {
		switch cd.Status {
		case "added":
			newCount++
		case "removed":
			oldCount++
		case "modified":
			oldCount++
			newCount++
		}
	}
	lines = append(lines, fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s", 1, oldCount, 1, newCount, td.Name))

	for _, cd := range td.ColumnDiffs {
		switch cd.Status {
		case "added":
			lines = append(lines, fmt.Sprintf("+  %s %s", cd.Name, cd.NewType))
		case "removed":
			lines = append(lines, fmt.Sprintf("-  %s %s", cd.Name, cd.OldType))
		case "modified":
			lines = append(lines, fmt.Sprintf("-  %s %s", cd.Name, cd.OldType))
			lines = append(lines, fmt.Sprintf("+  %s %s", cd.Name, cd.NewType))
		}
	}

	return strings.Join(lines, "\n")
}
