package openai

import (
	"fmt"
	"regexp"
	"strings"
)

// NormalizePartitionFilter rewrites all known DateTime-based date filters to
// proper Date range expressions so ClickHouse can apply partition pruning on:
//
//	mt_transaction_main  PARTITION BY toYYYYMM(created_at)   -- created_at is Date
//	mt_payment_archive   PARTITION BY toYYYYMM(created_at)   -- same schema
//
// The LLM frequently emits forms that bypass partition pruning:
//   - created_at_dt (wrong column name)
//   - created_at = yesterday() (point equality on DateTime vs Date)
//   - BETWEEN, toStartOfDay, subtractDays, addDays variants
//
// Safe to call on all generated SQL — all rewrites are no-ops when neither
// created_at_dt nor a point-equality on created_at appears in the SQL.
//
// Call order matters — more specific patterns must run before the column-rename
// fallback (step 8) so that BETWEEN/range forms are rewritten with correct
// semantics before a bare column rename would clobber them.
func NormalizePartitionFilter(sql string) string {
	// ── created_at_dt range rewrites ──────────────────────────────────────────
	// Rewrite directly to the final correct range — never produce an intermediate
	// point equality that would need a second rewrite pass.

	// 1. yesterday()
	sql = reYesterdayRange.ReplaceAllString(sql,
		"created_at >= toDateTime(yesterday()) AND created_at < toDateTime(today())")

	// 2. today()
	sql = reTodayRange.ReplaceAllString(sql,
		"created_at >= toDateTime(today()) AND created_at < toDateTime(today() + 1)")

	// 3. last N days: today() - N / subtractDays
	sql = rewriteLastNDays(sql)

	// 4. BETWEEN — must run before bare equality rewrites and column rename
	sql = rewriteBetween(sql)

	// 5. toStartOfDay variants
	sql = rewriteStartOfDay(sql)

	// 6. bare equality on created_at_dt
	sql = reBareYesterdayDt.ReplaceAllString(sql,
		"created_at >= toDateTime(yesterday()) AND created_at < toDateTime(today())")
	sql = reBareYesterdayDt2.ReplaceAllString(sql,
		"created_at >= toDateTime(yesterday()) AND created_at < toDateTime(today())")
	sql = rareBareToday.ReplaceAllString(sql,
		"created_at >= toDateTime(today()) AND created_at < toDateTime(today() + 1)")

	// 7. string literal DateTime ranges on created_at_dt
	//    e.g. created_at_dt >= '2024-03-01 00:00:00' AND created_at_dt < '2024-03-08 00:00:00'
	sql = rewriteStringLiteralRange(sql)

	// 8. column rename fallback — any remaining created_at_dt in WHERE/HAVING only.
	//    Scoped to WHERE/HAVING context to avoid clobbering SELECT aliases, ORDER BY,
	//    GROUP BY, and CTE column names.
	sql = rewriteColumnNameScoped(sql)

	// 9. point equality on created_at after column rename — catches anything the
	//    LLM emits directly as created_at = <date_func>() on partition tables.
	//    Only applied when the query targets a known partition table.
	if targetsPartitionTable(sql) {
		sql = rewritePointDateFilter(sql)
	}

	return sql
}

// ─── Package-level compiled regexes ──────────────────────────────────────────

var (
	// created_at_dt range: >= yesterday() AND < today()
	reYesterdayRange = regexp.MustCompile(
		`(?i)created_at_dt\s*>=\s*yesterday\(\)\s+AND\s+created_at_dt\s*<\s*today\(\)`,
	)

	// created_at_dt range: >= today() AND < today() + 1
	reTodayRange = regexp.MustCompile(
		`(?i)created_at_dt\s*>=\s*today\(\)\s+AND\s+created_at_dt\s*<\s*today\(\)\s*\+\s*1`,
	)

	// last N days: created_at_dt >= today() - N AND created_at_dt < today()
	reLastNDays = regexp.MustCompile(
		`(?i)created_at_dt\s*>=\s*today\(\)\s*-\s*(\d+)\s+AND\s+created_at_dt\s*<\s*today\(\)`,
	)

	// last N days via subtractDays: subtractDays(today(), N)
	reSubtractDays = regexp.MustCompile(
		`(?i)created_at_dt\s*>=\s*subtractDays\(today\(\)\s*,\s*(\d+)\)\s+AND\s+created_at_dt\s*<\s*today\(\)`,
	)

	// addDays upper bound variant: created_at_dt >= X AND created_at_dt < addDays(today(), 1)
	reAddDaysUpper = regexp.MustCompile(
		`(?i)created_at_dt\s*>=\s*today\(\)\s*-\s*(\d+)\s+AND\s+created_at_dt\s*<\s*addDays\(today\(\)\s*,\s*1\)`,
	)

	// BETWEEN: created_at_dt BETWEEN X AND Y
	reBetween = regexp.MustCompile(
		`(?i)created_at_dt\s+BETWEEN\s+([\w()',\-\s]+?)\s+AND\s+([\w()',\-\s]+?)(?:\s*(?:AND|OR|WHERE|GROUP|ORDER|LIMIT|HAVING|$|\)))`,
	)

	// toStartOfDay: created_at_dt >= toStartOfDay(yesterday()) AND created_at_dt < toStartOfDay(today())
	reStartOfDayYesterday = regexp.MustCompile(
		`(?i)created_at_dt\s*>=\s*toStartOfDay\(yesterday\(\)\)\s+AND\s+created_at_dt\s*<\s*toStartOfDay\(today\(\)\)`,
	)

	// toStartOfDay: created_at_dt >= toStartOfDay(today()) ...
	reStartOfDayToday = regexp.MustCompile(
		`(?i)created_at_dt\s*>=\s*toStartOfDay\(today\(\)\)\s+AND\s+created_at_dt\s*<\s*toStartOfDay\(today\(\)\s*\+\s*1\)`,
	)

	// bare equality on created_at_dt = yesterday()  (both orderings LLM uses)
	reBareYesterdayDt  = regexp.MustCompile(`(?i)created_at_dt\s*=\s*yesterday\(\)`)
	reBareYesterdayDt2 = regexp.MustCompile(`(?i)yesterday\(\)\s*=\s*created_at_dt`)
	rareBareToday      = regexp.MustCompile(`(?i)created_at_dt\s*=\s*today\(\)`)

	// string literal DateTime range on created_at_dt
	reStringLiteralRange = regexp.MustCompile(
		`(?i)created_at_dt\s*>=\s*'(\d{4}-\d{2}-\d{2}(?:\s+\d{2}:\d{2}:\d{2})?)'\s+AND\s+created_at_dt\s*<\s*'(\d{4}-\d{2}-\d{2}(?:\s+\d{2}:\d{2}:\d{2})?)'`,
	)

	// WHERE/HAVING clause detector — used to scope column rename
	reWhereClause  = regexp.MustCompile(`(?i)\bWHERE\b`)
	reHavingClause = regexp.MustCompile(`(?i)\bHAVING\b`)

	// Remaining created_at_dt in WHERE/HAVING context only
	reRemainingDtCol = regexp.MustCompile(`(?i)\bcreated_at_dt\b`)

	// Point equality on created_at (after column rename or direct from LLM)
	// Only applied when targetsPartitionTable() is true.
	rePointDateFilter = regexp.MustCompile(
		`(?i)\bcreated_at\s*=\s*(yesterday\(\)|today\(\)|toDate\([^)]+\))`,
	)
)

// partitionTables is the set of tables whose created_at column is a Date
// partition key. rewritePointDateFilter is scoped to these tables.
var partitionTables = map[string]bool{
	"mt_transaction_main": true,
	"mt_payment_archive":  true,
}

// ─── Rewrite helpers ──────────────────────────────────────────────────────────

func rewriteLastNDays(sql string) string {
	// today() - N
	sql = reLastNDays.ReplaceAllStringFunc(sql, func(match string) string {
		sub := reLastNDays.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return fmt.Sprintf("created_at >= today() - %s AND created_at < today()", sub[1])
	})

	// subtractDays(today(), N)
	sql = reSubtractDays.ReplaceAllStringFunc(sql, func(match string) string {
		sub := reSubtractDays.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return fmt.Sprintf("created_at >= today() - %s AND created_at < today()", sub[1])
	})

	// addDays upper bound
	sql = reAddDaysUpper.ReplaceAllStringFunc(sql, func(match string) string {
		sub := reAddDaysUpper.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return fmt.Sprintf("created_at >= today() - %s AND created_at < today()", sub[1])
	})

	return sql
}

func rewriteBetween(sql string) string {
	return reBetween.ReplaceAllStringFunc(sql, func(match string) string {
		sub := reBetween.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		lower := strings.TrimSpace(sub[1])
		upper := strings.TrimSpace(sub[2])
		// Preserve any trailing keyword that was captured by the lookahead group
		suffix := ""
		if len(sub) > 3 {
			suffix = sub[3]
		}
		rewritten := fmt.Sprintf("created_at >= toDateTime(%s) AND created_at < toDateTime(%s)", lower, upper)
		if suffix != "" {
			rewritten += " " + suffix
		}
		return rewritten
	})
}

func rewriteStartOfDay(sql string) string {
	sql = reStartOfDayYesterday.ReplaceAllString(sql,
		"created_at >= toDateTime(yesterday()) AND created_at < toDateTime(today())")
	sql = reStartOfDayToday.ReplaceAllString(sql,
		"created_at >= toDateTime(today()) AND created_at < toDateTime(today() + 1)")
	return sql
}

func rewriteStringLiteralRange(sql string) string {
	return reStringLiteralRange.ReplaceAllStringFunc(sql, func(match string) string {
		sub := reStringLiteralRange.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		lower := sub[1]
		upper := sub[2]
		return fmt.Sprintf("created_at >= toDateTime('%s') AND created_at < toDateTime('%s')", lower, upper)
	})
}

// rewriteColumnNameScoped renames remaining created_at_dt → created_at but only
// within WHERE and HAVING clauses, so SELECT aliases, ORDER BY, GROUP BY, and
// CTE column names are not touched.
func rewriteColumnNameScoped(sql string) string {
	upper := strings.ToUpper(sql)
	hasWhere := reWhereClause.MatchString(upper)
	hasHaving := reHavingClause.MatchString(upper)

	if !hasWhere && !hasHaving {
		// No WHERE/HAVING — any remaining created_at_dt is in SELECT/ORDER BY/GROUP BY;
		// do not rename blindly.
		return sql
	}

	// Replace only inside WHERE...  and HAVING... segments.
	// Strategy: split on clause keywords, rename in those segments, reassemble.
	return rewriteInClauses(sql, []string{"WHERE", "HAVING"})
}

// rewriteInClauses replaces created_at_dt → created_at only within the text
// following the given clause keywords, stopping at the next top-level keyword.
func rewriteInClauses(sql string, clauses []string) string {
	// Boundary keywords that end a WHERE/HAVING segment.
	endKeywords := regexp.MustCompile(
		`(?i)\b(GROUP\s+BY|ORDER\s+BY|LIMIT|OFFSET|HAVING|UNION|INTERSECT|EXCEPT|;\s*$)\b`,
	)

	for _, clause := range clauses {
		clauseRe := regexp.MustCompile(`(?i)\b` + clause + `\b`)
		loc := clauseRe.FindStringIndex(sql)
		if loc == nil {
			continue
		}

		start := loc[1] // character after the keyword
		rest := sql[start:]

		// Find where this clause ends.
		endLoc := endKeywords.FindStringIndex(rest)
		segmentLen := len(rest)
		if endLoc != nil {
			segmentLen = endLoc[0]
		}

		segment := rest[:segmentLen]
		rewritten := reRemainingDtCol.ReplaceAllString(segment, "created_at")
		sql = sql[:start] + rewritten + rest[segmentLen:]
	}

	return sql
}

// rewritePointDateFilter rewrites point equalities produced by the LLM directly
// or left over after column rename:
//
//	created_at = yesterday()  →  created_at >= toDateTime(yesterday()) AND created_at < toDateTime(today())
//	created_at = today()      →  created_at >= toDateTime(today()) AND created_at < toDateTime(today() + 1)
//	created_at = toDate(...)  →  created_at >= toDateTime(toDate(...)) AND created_at < toDateTime(toDate(...) + 1)
//
// Only called when the query targets a known partition table (see targetsPartitionTable).
func rewritePointDateFilter(sql string) string {
	return rePointDateFilter.ReplaceAllStringFunc(sql, func(match string) string {
		sub := rePointDateFilter.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		expr := sub[1]
		// Special-case: today() upper bound is today() + 1, yesterday() upper bound is today()
		switch strings.ToLower(strings.TrimSpace(expr)) {
		case "yesterday()":
			return "created_at >= toDateTime(yesterday()) AND created_at < toDateTime(today())"
		case "today()":
			return "created_at >= toDateTime(today()) AND created_at < toDateTime(today() + 1)"
		default:
			// toDate(...) or other expression
			return fmt.Sprintf(
				"created_at >= toDateTime(%s) AND created_at < toDateTime(%s + 1)",
				expr, expr,
			)
		}
	})
}

// targetsPartitionTable returns true when the SQL references at least one of the
// known partition tables. Used to gate rewritePointDateFilter so it does not
// incorrectly rewrite created_at on unrelated tables in JOINs or subqueries.
func targetsPartitionTable(sql string) bool {
	lower := strings.ToLower(sql)
	for table := range partitionTables {
		if strings.Contains(lower, table) {
			return true
		}
	}
	return false
}

// SQLContainsTable is a lightweight check used in tests and logging.
func SQLContainsTable(sql, table string) bool {
	return strings.Contains(strings.ToLower(sql), strings.ToLower(table))
}
