package graph

import (
	"testing"
)

func TestGraphRelationsAndTopologicalSort(t *testing.T) {
	g := NewGraph()

	// Topology:
	// users (parent)
	// categories (parent)
	// orders -> users (orders depends on users)
	// order_items -> orders
	// order_items -> products -> categories

	g.AddRelationship(Relationship{FromTable: "orders", FromColumn: "user_id", ToTable: "users", ToColumn: "id"})
	g.AddRelationship(Relationship{FromTable: "order_items", FromColumn: "order_id", ToTable: "orders", ToColumn: "id"})
	g.AddRelationship(Relationship{FromTable: "order_items", FromColumn: "product_id", ToTable: "products", ToColumn: "id"})
	g.AddRelationship(Relationship{FromTable: "products", FromColumn: "category_id", ToTable: "categories", ToColumn: "id"})

	// Check parents of orders -> should be users
	parents := g.GetParents("orders")
	if len(parents) != 1 || parents[0].ToTable != "users" {
		t.Fatalf("expected orders parent users, got %+v", parents)
	}

	// Check children of users -> should be orders
	children := g.GetChildren("users")
	if len(children) != 1 || children[0].FromTable != "orders" {
		t.Fatalf("expected users child orders, got %+v", children)
	}

	// Check topological sort order
	order := g.TopologicalSort()
	indexMap := make(map[string]int)
	for i, table := range order {
		indexMap[table] = i
	}

	// Parent MUST appear before child in topological insertion order
	if indexMap["users"] > indexMap["orders"] {
		t.Errorf("expected users before orders in topological order, got %v", order)
	}
	if indexMap["orders"] > indexMap["order_items"] {
		t.Errorf("expected orders before order_items, got %v", order)
	}
	if indexMap["categories"] > indexMap["products"] {
		t.Errorf("expected categories before products, got %v", order)
	}
}

func TestCycleDetection(t *testing.T) {
	g := NewGraph()

	// Cyclic loop:
	// departments.manager_id -> employees.id
	// employees.department_id -> departments.id
	g.AddRelationship(Relationship{FromTable: "departments", FromColumn: "manager_id", ToTable: "employees", ToColumn: "id"})
	g.AddRelationship(Relationship{FromTable: "employees", FromColumn: "department_id", ToTable: "departments", ToColumn: "id"})

	cycles := g.DetectCycles()
	if len(cycles) == 0 {
		t.Fatalf("expected cycle detected between departments and employees, got none")
	}

	// Self-loop test:
	// categories.parent_id -> categories.id
	g2 := NewGraph()
	g2.AddRelationship(Relationship{FromTable: "categories", FromColumn: "parent_id", ToTable: "categories", ToColumn: "id"})
	cycles2 := g2.DetectCycles()
	if len(cycles2) != 1 || cycles2[0][0] != "categories" {
		t.Fatalf("expected self-loop on categories, got %v", cycles2)
	}
}
