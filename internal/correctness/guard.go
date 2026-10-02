package correctness

import (
	"fmt"
	"strings"
	"sync"
)

// Guard tracks auto-increment seeds, foreign key cycle resolutions, and integrity safeguards.
type Guard struct {
	mu           sync.Mutex
	maxAutoIncID map[string]int64 // table -> max(id)
	repairSQLs   []string         // UPDATE statements to repair cycle references at the end
}

// NewGuard creates a new correctness guard.
func NewGuard() *Guard {
	return &Guard{
		maxAutoIncID: make(map[string]int64),
	}
}

// TrackID inspects an inserted ID to maintain the max auto-increment value for a table.
func (g *Guard) TrackID(table string, id any) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var num int64
	switch v := id.(type) {
	case int:
		num = int64(v)
	case int32:
		num = int64(v)
	case int64:
		num = v
	case uint:
		num = int64(v)
	case uint32:
		num = int64(v)
	case uint64:
		num = int64(v)
	default:
		return
	}

	normTable := strings.ToLower(table)
	if num > g.maxAutoIncID[normTable] {
		g.maxAutoIncID[normTable] = num
	}
}

// AddRepairStatement adds an UPDATE statement to be executed at file completion for cycle healing.
func (g *Guard) AddRepairStatement(stmt string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.repairSQLs = append(g.repairSQLs, stmt)
}

// GetMaxID returns the tracked maximum ID for a given table.
func (g *Guard) GetMaxID(table string) (int64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	maxID, ok := g.maxAutoIncID[strings.ToLower(table)]
	return maxID, ok
}

// GenerateSelfHealingSQL formats the auto-increment resets and cycle resolution SQL.
func (g *Guard) GenerateSelfHealingSQL(autoIncrTables []string) string {
	g.mu.Lock()
	defer g.mu.Unlock()

	var sb strings.Builder
	sb.WriteString("\n-- ========================================================\n")
	sb.WriteString("-- dbseal Correctness Guard: Self-Healing SQL\n")
	sb.WriteString("-- 1. Reset AUTO_INCREMENT seeds to max(id) + 1\n")
	sb.WriteString("-- 2. Apply cycle resolution updates\n")
	sb.WriteString("-- ========================================================\n\n")

	// 1. Auto-increment resets
	for _, table := range autoIncrTables {
		normTable := strings.ToLower(table)
		maxID, ok := g.maxAutoIncID[normTable]
		if ok && maxID > 0 {
			nextSeed := maxID + 1
			sb.WriteString(fmt.Sprintf("ALTER TABLE `%s` AUTO_INCREMENT = %d;\n", normTable, nextSeed))
		}
	}

	// 2. Cycle repair updates
	if len(g.repairSQLs) > 0 {
		sb.WriteString("\n-- Cycle Resolution Updates:\n")
		for _, stmt := range g.repairSQLs {
			stmt = strings.TrimSpace(stmt)
			if !strings.HasSuffix(stmt, ";") {
				stmt += ";"
			}
			sb.WriteString(stmt + "\n")
		}
	}

	return sb.String()
}
