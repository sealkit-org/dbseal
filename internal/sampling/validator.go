package sampling

import (
	"context"
	"fmt"
	"strings"

	"github.com/sealkit/dbseal/internal/config"
	"github.com/sealkit/dbseal/internal/drivers"
)

// InferredRelation represents a heuristic virtual foreign key candidate and its validation result.
type InferredRelation struct {
	FromTable    string
	FromColumn   string
	ToTable      string
	ToColumn     string
	MatchRate    float64 // 0.0 - 1.0 (e.g. 0.985 = 98.5%)
	SampledCount int
	MatchedCount int
	Status       string  // "confirmed" | "candidate" | "unverified"
	Reason       string
}

// Validator discovers and validates virtual foreign keys via sampling JOIN checks.
type Validator struct {
	driver drivers.Driver
	cfg    *config.Config
}

func NewValidator(driver drivers.Driver, cfg *config.Config) *Validator {
	return &Validator{
		driver: driver,
		cfg:    cfg,
	}
}

// DetectCandidates analyzes table schemas using naming heuristics to find potential virtual foreign keys.
func (v *Validator) DetectCandidates(ctx context.Context, tables []string) ([]InferredRelation, error) {
	schemas := make(map[string]*drivers.TableSchema)
	for _, t := range tables {
		norm := strings.ToLower(t)
		schema, err := v.driver.GetTableSchema(ctx, norm)
		if err != nil {
			continue
		}
		schemas[norm] = schema
	}

	var candidates []InferredRelation
	seen := make(map[string]bool)

	for fromTable, fromSchema := range schemas {
		for toTable, toSchema := range schemas {
			if fromTable == toTable {
				continue
			}

			// Target table must have a primary key (usually id)
			var toPK string
			if len(toSchema.PKColumns) > 0 {
				toPK = toSchema.PKColumns[0]
			} else {
				continue
			}

			possibleNames := PossibleFKColumnNames(toTable)
			for _, col := range fromSchema.Columns {
				for _, expected := range possibleNames {
					if strings.EqualFold(col.Name, expected) {
						key := fmt.Sprintf("%s.%s->%s.%s", fromTable, col.Name, toTable, toPK)
						if !seen[key] {
							seen[key] = true
							candidates = append(candidates, InferredRelation{
								FromTable:  fromTable,
								FromColumn: col.Name,
								ToTable:    toTable,
								ToColumn:   toPK,
								Status:     "candidate",
							})
						}
					}
				}
			}
		}
	}

	return candidates, nil
}

// ValidateRelation executes sampling validation on the source DB for a candidate.
func (v *Validator) ValidateRelation(ctx context.Context, rel InferredRelation, sampleLimit int, threshold float64) (InferredRelation, error) {
	if sampleLimit <= 0 {
		sampleLimit = 1000
	}
	if threshold <= 0 {
		threshold = 0.95
	}

	// 1. Sample distinct values from child table (using LIMIT without ORDER BY RAND to protect DB)
	sampleQuery := fmt.Sprintf("SELECT DISTINCT `%s` FROM `%s` WHERE `%s` IS NOT NULL LIMIT %d",
		rel.FromColumn, rel.FromTable, rel.FromColumn, sampleLimit)

	sampledIDs, err := v.driver.QueryIDs(ctx, sampleQuery)
	if err != nil {
		rel.Status = "unverified"
		rel.Reason = fmt.Sprintf("sampling query failed: %v", err)
		return rel, nil
	}

	rel.SampledCount = len(sampledIDs)
	if rel.SampledCount == 0 {
		rel.Status = "unverified"
		rel.Reason = "child table is empty or all foreign key values are NULL"
		return rel, nil
	}

	// 2. Query how many of these sampled IDs actually exist in the parent table
	idList := formatIDList(sampledIDs)
	matchQuery := fmt.Sprintf("SELECT `%s` FROM `%s` WHERE `%s` IN (%s)",
		rel.ToColumn, rel.ToTable, rel.ToColumn, idList)

	matchedIDs, err := v.driver.QueryIDs(ctx, matchQuery)
	if err != nil {
		rel.Status = "unverified"
		rel.Reason = fmt.Sprintf("parent lookup query failed: %v", err)
		return rel, nil
	}

	rel.MatchedCount = len(matchedIDs)
	rel.MatchRate = float64(rel.MatchedCount) / float64(rel.SampledCount)

	// 3. Evaluate against threshold
	if rel.MatchRate >= threshold {
		rel.Status = "confirmed"
		rel.Reason = fmt.Sprintf("match rate %.1f%% >= threshold %.1f%%", rel.MatchRate*100, threshold*100)
	} else {
		rel.Status = "candidate"
		rel.Reason = fmt.Sprintf("match rate %.1f%% < threshold %.1f%% (rejected from automatic traversal)", rel.MatchRate*100, threshold*100)
	}

	return rel, nil
}

// AutoDetectAndValidate detects all candidate virtual FKs and validates them via value sampling.
func (v *Validator) AutoDetectAndValidate(ctx context.Context, tables []string) ([]InferredRelation, error) {
	candidates, err := v.DetectCandidates(ctx, tables)
	if err != nil {
		return nil, err
	}

	samplingCfg := v.cfg.Relations.AutoDetect.Sampling
	sampleRows := samplingCfg.SampleRows
	threshold := samplingCfg.MatchThreshold

	var validated []InferredRelation
	for _, cand := range candidates {
		res, err := v.ValidateRelation(ctx, cand, sampleRows, threshold)
		if err != nil {
			continue
		}
		validated = append(validated, res)
	}

	return validated, nil
}

func formatIDList(ids []any) string {
	var parts []string
	for _, id := range ids {
		switch v := id.(type) {
		case string:
			parts = append(parts, fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''")))
		default:
			parts = append(parts, fmt.Sprintf("%v", v))
		}
	}
	if len(parts) == 0 {
		return "NULL"
	}
	return strings.Join(parts, ", ")
}
