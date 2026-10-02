package correctness

import (
	"strings"
	"testing"
)

func TestGuardAutoIncrementAndSelfHealing(t *testing.T) {
	guard := NewGuard()

	// Track IDs for users
	guard.TrackID("users", int64(100))
	guard.TrackID("users", 250)
	guard.TrackID("users", int64(180))

	// Track IDs for orders
	guard.TrackID("orders", uint64(5000))
	guard.TrackID("orders", uint64(5005))

	maxUsers, ok := guard.GetMaxID("users")
	if !ok || maxUsers != 250 {
		t.Fatalf("expected max users ID 250, got %d", maxUsers)
	}

	maxOrders, ok := guard.GetMaxID("orders")
	if !ok || maxOrders != 5005 {
		t.Fatalf("expected max orders ID 5005, got %d", maxOrders)
	}

	// Add cycle resolution update
	guard.AddRepairStatement("UPDATE departments SET manager_id = 12 WHERE id = 1")

	sql := guard.GenerateSelfHealingSQL([]string{"users", "orders"})

	// Verify AUTO_INCREMENT statements
	if !strings.Contains(sql, "ALTER TABLE `users` AUTO_INCREMENT = 251;") {
		t.Errorf("expected users AUTO_INCREMENT 251, got:\n%s", sql)
	}
	if !strings.Contains(sql, "ALTER TABLE `orders` AUTO_INCREMENT = 5006;") {
		t.Errorf("expected orders AUTO_INCREMENT 5006, got:\n%s", sql)
	}

	// Verify cycle repair statement
	if !strings.Contains(sql, "UPDATE departments SET manager_id = 12 WHERE id = 1;") {
		t.Errorf("expected cycle repair SQL, got:\n%s", sql)
	}
}
