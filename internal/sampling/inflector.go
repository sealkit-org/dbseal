package sampling

import (
	"strings"
)

// ToSingular converts common English plural table names to singular form.
// e.g. "users" -> "user", "categories" -> "category", "addresses" -> "address".
func ToSingular(plural string) string {
	lower := strings.ToLower(plural)

	if strings.HasSuffix(lower, "ies") && len(lower) > 3 {
		return lower[:len(lower)-3] + "y" // categories -> category, companies -> company
	}
	if strings.HasSuffix(lower, "sses") || strings.HasSuffix(lower, "shes") || strings.HasSuffix(lower, "ches") || strings.HasSuffix(lower, "xes") {
		return lower[:len(lower)-2] // addresses -> address, boxes -> box
	}
	if strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") && len(lower) > 1 {
		return lower[:len(lower)-1] // users -> user, orders -> order, items -> item
	}

	return lower
}

// PossibleFKColumnNames generates likely foreign key column names for a target table.
// e.g. for table "users", returns ["user_id", "users_id", "author_id", "created_by", "owner_id"].
func PossibleFKColumnNames(targetTable string) []string {
	singular := ToSingular(targetTable)
	var names []string

	names = append(names, singular+"_id")
	names = append(names, singular+"Id") // camelCase

	if singular != targetTable {
		names = append(names, targetTable+"_id")
	}

	// Semantic conventions for users/accounts
	if singular == "user" || singular == "account" {
		names = append(names, "created_by", "updated_by", "author_id", "owner_id", "creator_id")
	}

	return names
}
