package openai

import (
	"regexp"
	"strings"
)

// rewritePatterns holds compiled regexes for created_at_dt → created_at rewrites
// on tables where created_at (Date) is the partition key.
//
// mt_transaction_main  PARTITION BY toYYYYMM(created_at)
// mt_payment_archive   PARTITION BY toYYYYMM(created_at)  (same engine / schema pattern)
//
// The model frequently emits DateTime range filters like:
//
//	created_at_dt >= yesterday() AND created_at_dt < today()
//	created_at_dt >= today() - 7 AND created_at_dt < today()
//	created_at_dt >= today() - 30 AND created_at_dt < today()
//
// These bypass partition pruning and cause full 8B-row scans.
// We rewrite them to their Date equivalents before the query reaches ClickHouse.

var (
	// yesterday range  → created_at = yesterday()
	reYesterdayRange = regexp.MustCompile(
		`(?i)created_at_dt\s*>=\s*yesterday\(\)\s+AND\s+created_at_dt\s*<\s*today\(\)`,
	)

	// today()  → created_at = today()
	reTodayRange = regexp.MustCompile(
		`(?i)created_at_dt\s*>=\s*today\(\)\s+AND\s+created_at_dt\s*<\s*today\(\)\s*\+\s*1`,
	)

	// last N days range  → created_at >= today() - N
	// matches: created_at_dt >= today() - N AND created_at_dt < today()
	// (handled via rewriteLastNDays using submatch)

	// bare standalone filters (no upper bound companion)
	reBareYesterday = regexp.MustCompile(`(?i)created_at_dt\s*=\s*yesterday\(\)`)
	reBareToday     = regexp.MustCompile(`(?i)created_at_dt\s*=\s*today\(\)`)

	// any remaining created_at_dt reference (fallback — replace column name only)
	reRemainingDtCol = regexp.MustCompile(`(?i)\bcreated_at_dt\b`)
)

// NormalizePartitionFilter rewrites DateTime-based created_at_dt filters to
// Date-based created_at filters so that ClickHouse can apply partition pruning.
//
// Safe to call on all generated queries — rewrites are no-ops when
// created_at_dt does not appear in the SQL.
func NormalizePartitionFilter(sql string) string {
	// 1. yesterday range
	sql = reYesterdayRange.ReplaceAllString(sql, "created_at = yesterday()")

	// 2. today range (edge case)
	sql = reTodayRange.ReplaceAllString(sql, "created_at = today()")

	// 3. last N days range  →  created_at >= today() - N
	sql = rewriteLastNDays(sql)

	// 4. bare equality forms
	sql = reBareYesterday.ReplaceAllString(sql, "created_at = yesterday()")
	sql = reBareToday.ReplaceAllString(sql, "created_at = today()")

	// 5. fallback: any remaining created_at_dt → created_at
	//    (covers edge cases like >= / <= variants the model might invent)
	sql = reRemainingDtCol.ReplaceAllString(sql, "created_at")

	return sql
}

// rewriteLastNDays handles: created_at_dt >= today() - N AND created_at_dt < today()
// → created_at >= today() - N
// Uses FindAllStringSubmatchIndex to preserve surrounding SQL.
func rewriteLastNDays(sql string) string {
	pattern := regexp.MustCompile(
		`(?i)created_at_dt\s*>=\s*today\(\)\s*-\s*(\d+)\s+AND\s+created_at_dt\s*<\s*today\(\)`,
	)
	return pattern.ReplaceAllStringFunc(sql, func(match string) string {
		sub := pattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return "created_at >= today() - " + sub[1]
	})
}

// rewriteTablesNeedingPartitionFix is the set of tables where created_at is
// the Date partition key and created_at_dt must never be used for date-only
// filters. Kept here for documentation / future per-table gating if needed.
var rewriteTablesNeedingPartitionFix = map[string]bool{
	"mt_transaction_main": true,
	"mt_payment_archive":  true,
}

// SQLContainsTable is a lightweight check used in tests and logging.
func SQLContainsTable(sql, table string) bool {
	return strings.Contains(strings.ToLower(sql), strings.ToLower(table))
}
