package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/atterpac/qry/internal/config"
)

// PlanNode represents a node in a query execution plan.
type PlanNode struct {
	NodeType   string
	Relation   string
	TotalCost  float64
	PlanRows   int
	ActualRows int
	ActualTime float64
	IndexName  string
	Filter     string
	Children   []*PlanNode
	Extra      map[string]string // additional engine-specific details
}

// PlanResult holds the parsed execution plan.
type PlanResult struct {
	Root    *PlanNode
	RawText string // fallback raw EXPLAIN output
}

// ExplainQuery runs EXPLAIN on the given SQL and returns a parsed plan.
func ExplainQuery(ctx context.Context, provider Provider, sql string) (*PlanResult, error) {
	sql = strings.TrimSpace(sql)

	switch provider.EngineType() {
	case config.EnginePostgres:
		return explainPostgres(ctx, provider, sql)
	case config.EngineMySQL:
		return explainMySQL(ctx, provider, sql)
	case config.EngineSQLite:
		return explainSQLite(ctx, provider, sql)
	default:
		return explainFallback(ctx, provider, sql)
	}
}

// isSelectLike returns true if the query is safe for EXPLAIN ANALYZE.
func isSelectLike(sql string) bool {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
}

func explainPostgres(ctx context.Context, provider Provider, sql string) (*PlanResult, error) {
	var explainSQL string
	if isSelectLike(sql) {
		explainSQL = fmt.Sprintf("EXPLAIN (ANALYZE, FORMAT JSON) %s", sql)
	} else {
		explainSQL = fmt.Sprintf("EXPLAIN (FORMAT JSON) %s", sql)
	}

	result, err := provider.ExecuteQuery(ctx, explainSQL)
	if err != nil {
		return nil, err
	}

	if len(result.Rows) == 0 || len(result.Rows[0]) == 0 {
		return &PlanResult{RawText: "No plan data returned"}, nil
	}

	// Postgres JSON format returns a single row with all JSON
	jsonStr := result.Rows[0][0]
	// Sometimes rows are split across multiple rows
	if len(result.Rows) > 1 {
		var parts []string
		for _, row := range result.Rows {
			if len(row) > 0 {
				parts = append(parts, row[0])
			}
		}
		jsonStr = strings.Join(parts, "")
	}

	var plans []struct {
		Plan json.RawMessage `json:"Plan"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &plans); err != nil {
		return &PlanResult{RawText: jsonStr}, nil
	}

	if len(plans) == 0 {
		return &PlanResult{RawText: jsonStr}, nil
	}

	root := parsePgPlanNode(plans[0].Plan)
	if root == nil {
		return &PlanResult{RawText: jsonStr}, nil
	}

	return &PlanResult{Root: root, RawText: jsonStr}, nil
}

func parsePgPlanNode(raw json.RawMessage) *PlanNode {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}

	node := &PlanNode{
		Extra: make(map[string]string),
	}

	if v, ok := m["Node Type"]; ok {
		json.Unmarshal(v, &node.NodeType)
	}
	if v, ok := m["Relation Name"]; ok {
		json.Unmarshal(v, &node.Relation)
	}
	if v, ok := m["Total Cost"]; ok {
		json.Unmarshal(v, &node.TotalCost)
	}
	if v, ok := m["Plan Rows"]; ok {
		json.Unmarshal(v, &node.PlanRows)
	}
	if v, ok := m["Actual Rows"]; ok {
		json.Unmarshal(v, &node.ActualRows)
	}
	if v, ok := m["Actual Total Time"]; ok {
		json.Unmarshal(v, &node.ActualTime)
	}
	if v, ok := m["Index Name"]; ok {
		json.Unmarshal(v, &node.IndexName)
	}
	if v, ok := m["Filter"]; ok {
		json.Unmarshal(v, &node.Filter)
	}

	// Capture additional useful fields
	for _, key := range []string{"Join Type", "Sort Key", "Hash Cond", "Index Cond", "Recheck Cond", "Startup Cost", "Shared Hit Blocks", "Shared Read Blocks"} {
		if v, ok := m[key]; ok {
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				node.Extra[key] = s
			} else {
				node.Extra[key] = string(v)
			}
		}
	}

	if children, ok := m["Plans"]; ok {
		var childNodes []json.RawMessage
		if err := json.Unmarshal(children, &childNodes); err == nil {
			for _, child := range childNodes {
				if cn := parsePgPlanNode(child); cn != nil {
					node.Children = append(node.Children, cn)
				}
			}
		}
	}

	return node
}

func explainMySQL(ctx context.Context, provider Provider, sql string) (*PlanResult, error) {
	explainSQL := fmt.Sprintf("EXPLAIN %s", sql)
	result, err := provider.ExecuteQuery(ctx, explainSQL)
	if err != nil {
		return nil, err
	}

	if len(result.Rows) == 0 {
		return &PlanResult{RawText: "No plan data returned"}, nil
	}

	// Build raw text for fallback
	var rawLines []string
	rawLines = append(rawLines, strings.Join(result.Columns, "\t"))
	for _, row := range result.Rows {
		rawLines = append(rawLines, strings.Join(row, "\t"))
	}
	rawText := strings.Join(rawLines, "\n")

	// Build tree from MySQL EXPLAIN rows
	// Column indices: id, select_type, table, partitions, type, possible_keys, key, key_len, ref, rows, filtered, Extra
	root := &PlanNode{
		NodeType: "Query",
		Extra:    make(map[string]string),
	}

	for _, row := range result.Rows {
		child := &PlanNode{Extra: make(map[string]string)}

		if len(row) > 1 {
			child.NodeType = row[1] // select_type
		}
		if len(row) > 2 {
			child.Relation = row[2] // table
		}
		if len(row) > 4 {
			child.Extra["Access Type"] = row[4]
		}
		if len(row) > 6 {
			child.IndexName = row[6] // key
		}
		if len(row) > 9 {
			child.PlanRows, _ = strconv.Atoi(row[9])
		}
		if len(row) > 11 {
			child.Extra["Extra"] = row[11]
		}

		root.Children = append(root.Children, child)
	}

	return &PlanResult{Root: root, RawText: rawText}, nil
}

func explainSQLite(ctx context.Context, provider Provider, sql string) (*PlanResult, error) {
	explainSQL := fmt.Sprintf("EXPLAIN QUERY PLAN %s", sql)
	result, err := provider.ExecuteQuery(ctx, explainSQL)
	if err != nil {
		return nil, err
	}

	if len(result.Rows) == 0 {
		return &PlanResult{RawText: "No plan data returned"}, nil
	}

	// Build raw text
	var rawLines []string
	for _, row := range result.Rows {
		rawLines = append(rawLines, strings.Join(row, "\t"))
	}
	rawText := strings.Join(rawLines, "\n")

	// SQLite EXPLAIN QUERY PLAN returns: id, parent, notused, detail
	nodes := make(map[int]*PlanNode)
	var rootNode *PlanNode

	for _, row := range result.Rows {
		if len(row) < 4 {
			continue
		}

		id, _ := strconv.Atoi(row[0])
		parent, _ := strconv.Atoi(row[1])
		detail := row[3]

		node := &PlanNode{
			NodeType: detail,
			Extra:    make(map[string]string),
		}

		// Try to extract table name from detail
		if strings.Contains(detail, "TABLE") {
			parts := strings.Fields(detail)
			for i, p := range parts {
				if strings.EqualFold(p, "TABLE") && i+1 < len(parts) {
					node.Relation = parts[i+1]
					break
				}
			}
		}

		nodes[id] = node

		if parentNode, ok := nodes[parent]; ok && id != parent {
			parentNode.Children = append(parentNode.Children, node)
		} else if rootNode == nil {
			rootNode = node
		}
	}

	if rootNode == nil && len(nodes) > 0 {
		// Take the first node as root
		for _, n := range nodes {
			rootNode = n
			break
		}
	}

	return &PlanResult{Root: rootNode, RawText: rawText}, nil
}

func explainFallback(ctx context.Context, provider Provider, sql string) (*PlanResult, error) {
	explainSQL := fmt.Sprintf("EXPLAIN %s", sql)
	result, err := provider.ExecuteQuery(ctx, explainSQL)
	if err != nil {
		return nil, err
	}

	var rawLines []string
	if len(result.Columns) > 0 {
		rawLines = append(rawLines, strings.Join(result.Columns, "\t"))
	}
	for _, row := range result.Rows {
		rawLines = append(rawLines, strings.Join(row, "\t"))
	}

	return &PlanResult{RawText: strings.Join(rawLines, "\n")}, nil
}

// MaxCost returns the maximum TotalCost in the plan tree.
func (p *PlanResult) MaxCost() float64 {
	if p.Root == nil {
		return 0
	}
	return maxCostRecursive(p.Root)
}

func maxCostRecursive(n *PlanNode) float64 {
	max := n.TotalCost
	for _, child := range n.Children {
		if c := maxCostRecursive(child); c > max {
			max = c
		}
	}
	return max
}
