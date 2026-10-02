package graph

import (
	"strings"
	"sync"
)

// Relationship represents a directed foreign key or logical edge between two tables.
type Relationship struct {
	FromTable  string // e.g. orders
	FromColumn string // e.g. user_id
	ToTable    string // e.g. users
	ToColumn   string // e.g. id
	IsVirtual  bool   // true if virtual / inferred foreign key
	Condition  string // optional filter condition for polymorphic associations
}

// Graph models the relational topology of the database schema.
type Graph struct {
	mu           sync.RWMutex
	tables       map[string]bool
	outgoing     map[string][]Relationship // fromTable -> [rel: fromTable -> toTable] (Upstream: child -> parent)
	incoming     map[string][]Relationship // toTable -> [rel: fromTable -> toTable] (Downstream: parent -> child)
}

// NewGraph creates an empty relational graph.
func NewGraph() *Graph {
	return &Graph{
		tables:   make(map[string]bool),
		outgoing: make(map[string][]Relationship),
		incoming: make(map[string][]Relationship),
	}
}

// AddTable registers a table in the graph.
func (g *Graph) AddTable(table string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tables[strings.ToLower(table)] = true
}

// AddRelationship adds a foreign key relationship (child -> parent).
func (g *Graph) AddRelationship(rel Relationship) {
	g.mu.Lock()
	defer g.mu.Unlock()

	from := strings.ToLower(rel.FromTable)
	to := strings.ToLower(rel.ToTable)

	g.tables[from] = true
	g.tables[to] = true

	g.outgoing[from] = append(g.outgoing[from], rel)
	g.incoming[to] = append(g.incoming[to], rel)
}

// GetParents returns all upstream parent relationships (N:1) for a table.
// Example: for table "orders", returns orders.user_id -> users.id
func (g *Graph) GetParents(table string) []Relationship {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.outgoing[strings.ToLower(table)]
}

// GetChildren returns all downstream child relationships (1:N) pointing to this table.
// Example: for table "users", returns orders.user_id -> users.id
func (g *Graph) GetChildren(table string) []Relationship {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.incoming[strings.ToLower(table)]
}

// GetTables returns all known tables in the graph.
func (g *Graph) GetTables() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var list []string
	for t := range g.tables {
		list = append(list, t)
	}
	return list
}

// DetectCycles returns all Strongly Connected Components (SCC) with size > 1 or self-loops using Tarjan's algorithm.
func (g *Graph) DetectCycles() [][]string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var (
		index       = 0
		indices     = make(map[string]int)
		lowlink     = make(map[string]int)
		onStack     = make(map[string]bool)
		stack       []string
		cycles      [][]string
	)

	var strongConnect func(v string)
	strongConnect = func(v string) {
		indices[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for _, rel := range g.outgoing[v] {
			w := strings.ToLower(rel.ToTable)
			if !g.tables[w] {
				continue
			}

			if _, visited := indices[w]; !visited {
				strongConnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if indices[w] < lowlink[v] {
					lowlink[v] = indices[w]
				}
			}
		}

		if lowlink[v] == indices[v] {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			// SCC with more than 1 node is a cycle
			if len(scc) > 1 {
				cycles = append(cycles, scc)
			} else {
				// Check for self-loop (e.g. employees.manager_id -> employees.id)
				for _, rel := range g.outgoing[v] {
					if strings.EqualFold(rel.ToTable, v) {
						cycles = append(cycles, []string{v})
						break
					}
				}
			}
		}
	}

	for t := range g.tables {
		if _, visited := indices[t]; !visited {
			strongConnect(t)
		}
	}

	return cycles
}

// TopologicalSort computes an insertion order where parent tables appear before child tables.
// If cycles exist, a best-effort order is returned.
func (g *Graph) TopologicalSort() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// In-degree from parent perspective: an edge child -> parent means parent must be inserted first
	// Dependency: child depends on parent
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // parent -> []children

	for t := range g.tables {
		inDegree[t] = 0
	}

	for child, parents := range g.outgoing {
		for _, rel := range parents {
			parent := strings.ToLower(rel.ToTable)
			if g.tables[parent] && parent != child {
				inDegree[child]++
				dependents[parent] = append(dependents[parent], child)
			}
		}
	}

	var queue []string
	for t, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, t)
		}
	}

	var order []string
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)

		for _, dep := range dependents[curr] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	// Add any remaining nodes (part of cycles)
	seen := make(map[string]bool)
	for _, t := range order {
		seen[t] = true
	}
	for t := range g.tables {
		if !seen[t] {
			order = append(order, t)
		}
	}

	return order
}
