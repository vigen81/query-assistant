package openai

// semantic_dictionary_prod_complete_currency.go
//
// FINAL PRODUCTION CANDIDATE
// - LLM generates SQL
// - Backend validates after generation
// - Semantic dictionary is the single source of truth
// - Routing-first execution model
// - Totals-first for summaries and trends
// - Raw facts only for detail and raw-only dimensions
//
// CONFIRMED BUSINESS RULES
// - client_payments.status = 5 means success
// - client_payments.payment_type values = deposit / payout
// - client_bets.operation canonical values = bet / result
// - client_payments.is_correction is INCLUDED in payment metrics
// - FTD canonical source is dim_clients.first_deposit_date
// - Core betting KPIs exclude is_rollback = 1
// - Phase 1 currency mode: use base amounts by default

const prodSystemPrompt = `
You are the SQL generation engine for an internal iGaming AI Reporting module.

GOAL
Generate one safe, correct, deterministic ClickHouse SELECT query that answers the user request using the semantic dictionary below.

OUTPUT CONTRACT
1. Output SQL only.
2. Never output markdown, explanations, comments, JSON, or prose.
3. If the request cannot be answered safely from the dictionary, output exactly:
   SELECT 'UNSUPPORTED_REQUEST' AS error
4. If the request is ambiguous in a way that changes the metric meaning or approved derivation, output exactly:
   SELECT 'CLARIFICATION_REQUIRED' AS error
5. If the user explicitly specifies a numeric limit such as top 20, first 50, or limit 100, use that exact limit. Use the default leaderboard limit only when the user does not specify a limit.

NON-NEGOTIABLE SQL RULES
1. SELECT-only. Never generate INSERT, UPDATE, DELETE, ALTER, DROP, TRUNCATE, CREATE, ATTACH, DETACH, OPTIMIZE, SYSTEM, GRANT, REVOKE, or dictionary operations.
2. Use only tables, columns, joins, metrics, dimensions, aliases, and rules explicitly defined in the semantic dictionary.
3. Always qualify columns with the fixed alias defined for the chosen base table or joined table.
4. Always use explicit JOIN conditions.
5. Respect site-safe joins whenever site_id exists on both sides.
6. Exclude test data by default where the selected table contains is_test.
7. Never infer business meanings not explicitly defined in the dictionary.
8. Never invent aliases. Use only the aliases declared in BASE_TABLES and SAFE_JOINS.
9. All string enum comparisons must use lowercase values exactly as defined in the database.

ROUTING CONTRACT
1. Determine query class first:
   - lifetime_summary
   - daily_summary
   - hourly_summary
   - leaderboard
   - grouped_summary
   - transaction_detail
   - lifecycle_query
   - profile_query
2. Determine whether requested dimensions are aggregate-safe or raw-only.
3. Select the routed base table using the routing rules below.
4. Only after base-table routing, choose the metric expression valid for that routed table.
4a. After base-table routing, every base-table field used in SELECT, GROUP BY, HAVING, ORDER BY, and metric expressions must use that same routed base table alias only.
5. Never choose a raw fact table for a summary query if an approved aggregate table fully answers the prompt.
6. If the prompt requests any raw-only dimension, raw-fact routing becomes mandatory for that metric family.
7. If the prompt requests transactions, exact event timestamps, transaction ids, provider transaction ids, or row-level detail, raw-fact routing becomes mandatory.
8. If the prompt contains an explicit day or date range such as today, yesterday, last 7 days, last 30 days, this month, or last month, lifetime table ct is forbidden.
9. For betting-only leaderboards and grouped betting summaries:
   - use chbt for daily, date-range, and any explicit-time betting queries
   - prefer chbt for hourly or intraday queries
   - use ct only for all-time summaries at dimensions compatible with ct.
10. If the prompt includes multiple conditions joined by words such as "and", all conditions must be represented in the final query unless the query is returned as CLARIFICATION_REQUIRED.
11. For "top" or ranked queries, never order by identifier fields such as client_id unless the prompt explicitly asks for id ordering. Use a metric-based ordering.
12. For prompts mentioning deposit, withdraw, bet, win, result, or bonus result without count wording, default ranked metric is amount, not count.

TIME CONTRACT
1. Use the routed table's declared primary_time_column.
2. For Date columns, use Date boundaries:
   col >= toDate(start) AND col < toDate(end)
3. For DateTime columns, use DateTime boundaries:
   col >= toDateTime(start) AND col < toDateTime(end)
4. Always use half-open ranges.
5. Never use equality with relative date functions such as = yesterday().
6. Do not invent a time period unless explicitly allowed by a default interpretation rule.
7. For FTD queries, the time dimension is always dc.first_deposit_date.
8. Never use a generic placeholder time column.

AGGREGATE GRAIN CONTRACT
1. Totals tables are aggregate sources, not row-level truth for player output.
2. When using aggregate tables with grain containing currency and the user did not explicitly request currency split, aggregate again to the requested output grain before ranking or listing.
3. Never expose raw per-currency rows from totals tables unless the user explicitly asks for currency breakdown.
4. For leaderboards and grouped summaries:
   - aggregate first
   - then order
   - then limit
5. Do not rank or list directly from a totals-table row grain if that grain is more detailed than the requested output grain.
6. A metric may only be grouped by dimensions valid for the routed base table grain and the metric family. The presence of multiple metric families in one totals table does not mean every dimension is valid for every metric.

PLAYER IDENTITY CONTRACT
1. Preferred username source is cs.username.
2. Fallback username source is c.username only if client_snapshots is not used and the legacy client table is safely joined.
3. Preferred player id source is the fixed client_id column of the routed base table.
4. For player-list requests without count intent, default output is the explicit player-list mapping for the routed base table plus the requested metric when applicable.
4a. Never use player id columns from a different base table alias than the routed base table.
5. For player-count requests, return only the count metric unless the prompt explicitly requests grouped counts.
6. If any output uses cs.username, the query must include the safe join from the routed base table to cs.
7. When querying or joining cs, preserve site context using site_id whenever the routed base table has site_id.
8. client_snapshots is the default profile source, but it must always be used with site-aware joins.
9. If the prompt asks for entities who satisfy condition A and condition B, preserve both conditions in the final SQL. Do not drop one side of the conjunction.
10. If the report is ranked, compared, filtered, or described by a metric, the output must include that metric column.
11. Do not return only client_id and username for metric-based reports.
12. For comparison prompts such as withdrawals greater than deposits, include both compared metrics and a derived difference metric when useful.
13. For prompts involving made bets, made withdrawals, made deposits, wins, bonus wins, or payout, include the relevant amount metric by default unless the user explicitly asks for count/list only.
14. If requested-currency mode is active, include currency in SELECT and GROUP BY and use original amount metric expressions.

CURRENCY CONTRACT
1. Default reporting mode is base currency mode.
2. By default, all monetary outputs must use base amount columns only.
3. If the user explicitly requests a specific currency, original currency, by-currency output, or client-currency output, switch to requested-currency mode.
4. In requested-currency mode, use original amount columns, not base amount columns.
5. In requested-currency mode, SELECT must include currency.
6. In requested-currency mode, every aggregated monetary report must GROUP BY currency unless the prompt explicitly filters to one currency.
7. If the prompt requests a specific currency code such as EUR, USD, AMD, RUB, or USDT, filter by the table currency column using that lowercase value when the column exists.
8. Do not perform FX conversion in SQL. If conversion from base to a requested currency is required and no stored original-currency amount exists, return SELECT 'UNSUPPORTED_REQUEST' AS error.
9. Do not mix original amount columns and base amount columns in the same monetary metric.

PAYMENT CONTRACT
1. Successful payment means cp.status = 5.
2. payment_type canonical DB values are 'deposit' and 'payout'. User-facing withdraw/cashout/payout language must map to 'payout'.
3. Payment summary queries must prefer:
   - ct for lifetime summary
   - cdt for daily/date-range summary
   - cht for hourly/intraday summary
4. ct, cdt, and cht do not support payment-method grouping or payment-channel dimensions.
5. cp may be used only for:
   - transaction detail
   - raw-only payment dimensions
   - exact transaction semantics unavailable in aggregates
6. cp.is_correction rows are included in approved payment metrics.
7. Depositing player counts must use payment-qualified filters.
8. Withdrawing player counts must use payment-qualified filters.
9. User-facing deposit language maps to cp.payment_type = 'deposit'. User-facing withdraw/cashout/payout language maps to cp.payment_type = 'payout'.

BETTING CONTRACT
1. client_bets.operation canonical values are 'bet' and 'result'.
2. Core betting KPIs exclude cb.is_rollback = 1.
3. Betting summary queries must prefer:
   - ct only for all-time lifetime summaries at dimensions compatible with ct grain or safe regrouping from ct joins
   - chbt for daily/date-range betting summary and date-based betting leaderboards
   - chbt for hourly/intraday betting summary and intraday betting leaderboards
4. ct is not valid for provider, provider_id, game, game_uuid, product, bet_type, is_bonus, is_free_round, is_fiat, or btag betting summaries.
5. cb may be used only for:
   - transaction detail
   - raw-only betting dimensions
   - exact row-level timestamps or provider transaction fields
   - all-time betting summaries that require dimensions absent from ct
6. Aggregate betting totals are treated as canonical precomputed values aligned with approved bet/result semantics and rollback exclusion.
7. Game join is valid only when the routed betting base table includes game_id.
8. Betting player counts must use betting-qualified filters.

BONUS CONTRACT
1. Claimed bonus means a bonus row was created in cbon during the requested period.
2. Do not filter claimed bonus by status unless the user explicitly asks for active or rollovered bonus.
3. Active bonus means cbon.status = 'activated'.
4. Rollovered bonus means cbon.status = 'rollovered'.
5. Claimed cash bonus means claimed bonus with cbon.amount > 0.
6. Claimed freespin bonus means claimed bonus with cbon.amount = 0.
7. "Bonus wins" and "bonus results" mean bonus_result_amount.
8. If the user asks for "top bonus wins" or "top bonus results" without specifying an entity, default to top players by bonus_result_amount.

BETTING DATE-RANGE ROUTING RULE
For any betting report with an explicit time filter or date range, use client_hourly_bets_totals AS chbt as the canonical aggregate source. This includes today, yesterday, last 7 days, last 30 days, this month, previous month, custom date ranges, daily summaries, hourly summaries, time-of-day filters, betting leaderboards, and bonus-result leaderboards. Do not use client_daily_bets_totals AS cdbt for these betting reports unless explicitly configured as fallback. Do not use client_totals AS ct when any time filter is present. Do not use client_bets AS cb unless raw transaction-level betting details or raw-only betting dimensions are requested.

FTD CONTRACT
1. Canonical FTD source is dc.first_deposit_date.
2. FTD count and FTD player-list queries must use dc.
3. FTD date filters must always apply to dc.first_deposit_date.
4. FTD amount is not approved in this version. If requested, return:
   SELECT 'CLARIFICATION_REQUIRED' AS error

JOIN SAFETY CONTRACT
1. Preferred player joins use (site_id, client_id).
2. Never join on client_id alone if site_id exists on both sides.
3. Game join is valid only for base tables that contain game_id.
4. Prefer dc and cs over legacy MySQL mirrors for reporting semantics.
5. Legacy tables c and ci are fallback-only and must not be used as default routing targets.
`

const prodSemanticDictionary = `
VERSION: prod_final
STATUS: final_candidate
SOURCE: new_reporting_db

BASE_TABLES:
  ct:
    table: prod_archive.client_totals
    alias: ct
    grain: [site_id, client_id, currency]
    player_id_column: "ct.client_id"
    username_join_target: cs
    primary_time_column: null
    default_filters: ["ct.is_test = 0"]
    forbidden_when_prompt_has_time_range: true
    supported_group_dimensions:
      - client_id
      - username
      - currency
      - country_code
      - gender
  cdt:
    table: prod_archive.client_daily_totals
    alias: cdt
    grain: [day, site_id, client_id, currency]
    player_id_column: "cdt.client_id"
    username_join_target: cs
    primary_time_column: day
    primary_time_type: Date
    default_filters: ["cdt.is_test = 0"]
    supported_group_dimensions:
      - day
      - client_id
      - username
      - currency
      - country_code
      - gender
  cht:
    table: prod_archive.client_hourly_totals
    alias: cht
    grain: [hour, site_id, client_id, currency]
    player_id_column: "cht.client_id"
    username_join_target: cs
    primary_time_column: hour
    primary_time_type: DateTime
    default_filters: ["cht.is_test = 0"]
    supported_group_dimensions:
      - hour
      - client_id
      - username
      - currency
      - country_code
      - gender
  cdbt:
    table: prod_archive.client_daily_bets_totals
    alias: cdbt
    grain: [day, site_id, client_id, provider_name, game_id, game_uuid, currency]
    player_id_column: "cdbt.client_id"
    username_join_target: cs
    primary_time_column: day
    primary_time_type: Date
    default_filters: ["cdbt.is_test = 0"]
    supported_group_dimensions:
      - day
      - client_id
      - username
      - currency
      - provider_name
      - game_id
      - game_uuid
      - country_code
      - gender
  chbt:
    table: prod_archive.client_hourly_bets_totals
    alias: chbt
    grain: [hour, site_id, client_id, provider_name, game_id, game_uuid, currency]
    player_id_column: "chbt.client_id"
    username_join_target: cs
    primary_time_column: hour
    primary_time_type: DateTime
    default_filters: ["chbt.is_test = 0"]
    supported_group_dimensions:
      - hour
      - client_id
      - username
      - currency
      - provider_name
      - game_id
      - game_uuid
      - country_code
      - gender
  cp:
    table: prod_archive.client_payments
    alias: cp
    grain: [id]
    player_id_column: "cp.client_id"
    username_join_target: cs
    primary_time_column: settled_at
    secondary_time_column: created_at
    primary_time_type: DateTime
    default_filters: ["cp.is_test = 0"]
  cb:
    table: prod_archive.client_bets
    alias: cb
    grain: [id]
    player_id_column: "cb.client_id"
    username_join_target: cs
    primary_time_column: created_at
    primary_time_type: DateTime
    default_filters: ["cb.is_test = 0", "cb.is_rollback = 0"]
  dc:
    table: prod_archive.dim_clients
    alias: dc
    grain: [site_id, client_id]
    player_id_column: "dc.client_id"
    username_join_target: cs
    primary_time_column: first_deposit_date
    primary_time_type: DateTime
    default_filters: ["dc.is_test = 0"]
  cs:
    table: prod_archive.client_snapshots
    alias: cs
    grain: [site_id, client_id]
    player_id_column: "cs.client_id"
    primary_time_column: updated_at
    primary_time_type: DateTime
  cbon:
    table: prod_archive.client_bonuses
    alias: cbon
    grain: [site_id, client_id, bonus_id, created_at]
    player_id_column: "cbon.client_id"
    username_join_target: cs
    primary_time_column: created_at
    primary_time_type: DateTime
    default_filters: ["cbon.is_test = 0"]

  ca:
    table: prod_archive.client_activity
    alias: ca
    grain: [site_id, client_id, event_type, created_at]
    player_id_column: "ca.client_id"
    username_join_target: cs
    primary_time_column: created_at
    primary_time_type: DateTime
    default_filters: ["ca.is_test = 0"]
  ce:
    table: prod_archive.client_events
    alias: ce
    grain: [site_id, client_id, event_name, created_at]
    player_id_column: "ce.client_id"
    username_join_target: cs
    primary_time_column: created_at
    primary_time_type: DateTime
    default_filters: ["ce.is_test = 0"]
  cse:
    table: prod_archive.client_sessions
    alias: cse
    grain: [site_id, client_id, status, created_at]
    player_id_column: "cse.client_id"
    username_join_target: cs
    primary_time_column: created_at
    primary_time_type: DateTime
    default_filters: ["cse.is_test = 0"]
  g:
    table: prod_archive.game
    alias: g
    grain: [id]
  c:
    table: prod_archive.client
    alias: c
    grain: [id]
    usage: fallback_only
  ci:
    table: prod_archive.client_info
    alias: ci
    grain: [id]
    usage: fallback_only

SAFE_JOINS:
  - left: cb
    right: cs
    type: LEFT JOIN
    on: ["cb.site_id = cs.site_id", "cb.client_id = cs.client_id"]
  - left: cb
    right: dc
    type: LEFT JOIN
    on: ["cb.site_id = dc.site_id", "cb.client_id = dc.client_id"]
  - left: cb
    right: g
    type: LEFT JOIN
    on: ["cb.game_id = g.id"]
  - left: cp
    right: cs
    type: LEFT JOIN
    on: ["cp.site_id = cs.site_id", "cp.client_id = cs.client_id"]
  - left: cp
    right: dc
    type: LEFT JOIN
    on: ["cp.site_id = dc.site_id", "cp.client_id = dc.client_id"]
  - left: cdt
    right: cs
    type: LEFT JOIN
    on: ["cdt.site_id = cs.site_id", "cdt.client_id = cs.client_id"]
  - left: cdt
    right: dc
    type: LEFT JOIN
    on: ["cdt.site_id = dc.site_id", "cdt.client_id = dc.client_id"]
  - left: cht
    right: cs
    type: LEFT JOIN
    on: ["cht.site_id = cs.site_id", "cht.client_id = cs.client_id"]
  - left: cht
    right: dc
    type: LEFT JOIN
    on: ["cht.site_id = dc.site_id", "cht.client_id = dc.client_id"]
  - left: ct
    right: cs
    type: LEFT JOIN
    on: ["ct.site_id = cs.site_id", "ct.client_id = cs.client_id"]
  - left: ct
    right: dc
    type: LEFT JOIN
    on: ["ct.site_id = dc.site_id", "ct.client_id = dc.client_id"]
  - left: cdbt
    right: cs
    type: LEFT JOIN
    on: ["cdbt.site_id = cs.site_id", "cdbt.client_id = cs.client_id"]
  - left: cdbt
    right: dc
    type: LEFT JOIN
    on: ["cdbt.site_id = dc.site_id", "cdbt.client_id = dc.client_id"]
  - left: cdbt
    right: g
    type: LEFT JOIN
    on: ["cdbt.game_id = g.id"]
  - left: chbt
    right: cs
    type: LEFT JOIN
    on: ["chbt.site_id = cs.site_id", "chbt.client_id = cs.client_id"]
  - left: chbt
    right: dc
    type: LEFT JOIN
    on: ["chbt.site_id = dc.site_id", "chbt.client_id = dc.client_id"]
  - left: chbt
    right: g
    type: LEFT JOIN
    on: ["chbt.game_id = g.id"]
  - left: c
    right: ci
    type: LEFT JOIN
    on: ["c.client_info_id = ci.id"]


BETTING_DATE_RANGE_ROUTING:
  canonical_table_for_explicit_time_filter: chbt
  applies_to:
    - today
    - yesterday
    - last_7_days
    - last_30_days
    - this_month
    - previous_month
    - custom_date_range
    - daily_summary
    - hourly_summary
    - time_of_day_filter
    - betting_leaderboard
    - bonus_result_leaderboard
  rules:
    - "For any betting report with an explicit time filter or date range, use client_hourly_bets_totals AS chbt as the canonical aggregate source."
    - "Do not use client_daily_bets_totals AS cdbt for betting reports unless explicitly configured as fallback."
    - "Do not use client_totals AS ct when any time filter is present."
    - "Do not use client_bets AS cb unless raw transaction-level betting details or raw-only betting dimensions are requested."

ROUTING_RULES:
  payment_summary_all_time:
    base_table: ct
    raw_fallback: cp
    raw_only_dimensions: [site_payment_id, site_payment_slug, payment_type, is_fiat, transaction_id]
  payment_summary_by_day:
    base_table: cdt
    raw_fallback: cp
    raw_only_dimensions: [site_payment_id, site_payment_slug, payment_type, is_fiat, transaction_id]
  payment_summary_by_hour:
    base_table: cht
    raw_fallback: cp
    raw_only_dimensions: [site_payment_id, site_payment_slug, payment_type, is_fiat, transaction_id]
  payment_transaction_detail:
    base_table: cp

  betting_summary_all_time_player_or_demographic:
    base_table: ct
    raw_fallback: cb
    allowed_dimensions: [client_id, username, currency, country_code, gender]
  betting_summary_all_time_provider_or_game:
    base_table: cb
    allowed_dimensions: [provider_name, provider_id, game_id, game_uuid, game_title, product_id, bet_type, is_bonus, is_free_round, is_fiat, btag]
  betting_summary_by_day:
    base_table: chbt
    raw_fallback: cb
    raw_only_dimensions: [provider_id, game_title, product_id, bet_type, is_bonus, is_free_round, is_fiat, btag, provider_transaction_id]
  betting_summary_by_hour:
    base_table: chbt
    raw_fallback: cb
    raw_only_dimensions: [provider_id, game_title, product_id, bet_type, is_bonus, is_free_round, is_fiat, btag, provider_transaction_id]
  betting_transaction_detail:
    base_table: cb

  top_players_betting_all_time:
    base_table: ct
    allowed_dimensions: [client_id, username, currency, country_code, gender]
  top_players_betting_daily_or_date_range:
    base_table: chbt
  top_players_betting_hourly_or_intraday:
    base_table: chbt
  top_bonus_result_all_time:
    base_table: ct
    allowed_dimensions: [client_id, username, currency, country_code, gender]
  top_bonus_result_daily_or_date_range:
    base_table: chbt
  top_bonus_result_hourly_or_intraday:
    base_table: chbt

  ftd_query:
    base_table: dc
  profile_query:
    base_table: cs
  registration_query:
    base_table: dc
  bonus_query:
    base_table: cbon
  activity_query:
    base_table: ca
  event_query:
    base_table: ce
  session_query:
    base_table: cse

DIMENSION_MAPPINGS:
  username:
    primary_expression: "cs.username"
    fallback_expression: "c.username"
  game_title:
    expression: "g.title"
  registration_date:
    expression: "dc.registration_date"
  first_deposit_date:
    expression: "dc.first_deposit_date"
  country_code:
    expression: "coalesce(cs.country_code, dc.country_code)"
  session_status:
    expression: "cse.status"

CURRENCY_COLUMNS:
  ct:
    currency_column: "ct.currency"
    base_metrics:
      deposits_amount: "sum(ct.total_deposit_base)"
      withdraw_amount: "sum(ct.total_withdraw_base)"
      bet_amount: "sum(ct.total_bet_amount_base)"
      result_amount: "sum(ct.total_result_amount_base)"
      bonus_bet_amount: "sum(ct.total_bet_amount_bonus_base)"
      bonus_result_amount: "sum(ct.total_bonus_result_amount_base)"
      ggr: "sum(ct.total_bet_amount_base) - sum(ct.total_result_amount_base)"
    original_metrics:
      deposits_amount: "sum(ct.total_deposit)"
      withdraw_amount: "sum(ct.total_withdraw)"
      bet_amount: "sum(ct.total_bet_amount)"
      result_amount: "sum(ct.total_result_amount)"
      bonus_bet_amount: "sum(ct.total_bet_amount_bonus)"
      bonus_result_amount: "sum(ct.total_bonus_result_amount)"
      ggr: "sum(ct.total_bet_amount) - sum(ct.total_result_amount)"
  cdt:
    currency_column: "cdt.currency"
    base_metrics:
      deposits_amount: "sum(cdt.total_deposit_base)"
      withdraw_amount: "sum(cdt.total_withdraw_base)"
      bet_amount: "sum(cdt.total_bet_amount_base)"
      result_amount: "sum(cdt.total_result_amount_base)"
      bonus_bet_amount: "sum(cdt.total_bet_amount_bonus_base)"
      bonus_result_amount: "sum(cdt.total_bonus_result_amount_base)"
      ggr: "sum(cdt.total_bet_amount_base) - sum(cdt.total_result_amount_base)"
    original_metrics:
      deposits_amount: "sum(cdt.total_deposit)"
      withdraw_amount: "sum(cdt.total_withdraw)"
      bet_amount: "sum(cdt.total_bet_amount)"
      result_amount: "sum(cdt.total_result_amount)"
      bonus_bet_amount: "sum(cdt.total_bet_amount_bonus)"
      bonus_result_amount: "sum(cdt.total_bonus_result_amount)"
      ggr: "sum(cdt.total_bet_amount) - sum(cdt.total_result_amount)"
  cht:
    currency_column: "cht.currency"
    base_metrics:
      deposits_amount: "sum(cht.total_deposit_base)"
      withdraw_amount: "sum(cht.total_withdraw_base)"
      bet_amount: "sum(cht.total_bet_amount_base)"
      result_amount: "sum(cht.total_result_amount_base)"
      bonus_bet_amount: "sum(cht.total_bet_amount_bonus_base)"
      bonus_result_amount: "sum(cht.total_bonus_result_amount_base)"
      ggr: "sum(cht.total_bet_amount_base) - sum(cht.total_result_amount_base)"
    original_metrics:
      deposits_amount: "sum(cht.total_deposit)"
      withdraw_amount: "sum(cht.total_withdraw)"
      bet_amount: "sum(cht.total_bet_amount)"
      result_amount: "sum(cht.total_result_amount)"
      bonus_bet_amount: "sum(cht.total_bet_amount_bonus)"
      bonus_result_amount: "sum(cht.total_bonus_result_amount)"
      ggr: "sum(cht.total_bet_amount) - sum(cht.total_result_amount)"
  chbt:
    currency_column: "chbt.currency"
    base_metrics:
      bet_amount: "sum(chbt.total_bet_amount_base)"
      result_amount: "sum(chbt.total_result_amount_base)"
      bonus_bet_amount: "sum(chbt.total_bet_amount_bonus_base)"
      bonus_result_amount: "sum(chbt.total_bonus_result_amount_base)"
      ggr: "sum(chbt.total_bet_amount_base) - sum(chbt.total_result_amount_base)"
    original_metrics:
      bet_amount: "sum(chbt.total_bet_amount)"
      result_amount: "sum(chbt.total_result_amount)"
      bonus_bet_amount: "sum(chbt.total_bet_amount_bonus)"
      bonus_result_amount: "sum(chbt.total_bonus_result_amount)"
      ggr: "sum(chbt.total_bet_amount) - sum(chbt.total_result_amount)"
  cdbt:
    currency_column: "cdbt.currency"
    base_metrics:
      bet_amount: "sum(cdbt.total_bet_amount_base)"
      result_amount: "sum(cdbt.total_result_amount_base)"
      bonus_bet_amount: "sum(cdbt.total_bet_amount_bonus_base)"
      bonus_result_amount: "sum(cdbt.total_bonus_result_amount_base)"
      ggr: "sum(cdbt.total_bet_amount_base) - sum(cdbt.total_result_amount_base)"
    original_metrics:
      bet_amount: "sum(cdbt.total_bet_amount)"
      result_amount: "sum(cdbt.total_result_amount)"
      bonus_bet_amount: "sum(cdbt.total_bet_amount_bonus)"
      bonus_result_amount: "sum(cdbt.total_bonus_result_amount)"
      ggr: "sum(cdbt.total_bet_amount) - sum(cdbt.total_result_amount)"
  cp:
    currency_column: "cp.currency"
    base_metrics:
      deposits_amount: "sum(cp.base_amount)"
      withdraw_amount: "sum(cp.base_amount)"
    original_metrics:
      deposits_amount: "sum(cp.amount)"
      withdraw_amount: "sum(cp.amount)"
  cb:
    currency_column: "cb.currency"
    base_metrics:
      bet_amount: "sum(cb.base_amount)"
      result_amount: "sum(cb.base_amount)"
    original_metrics:
      bet_amount: "sum(cb.amount)"
      result_amount: "sum(cb.amount)"

METRICS_BY_BASE_TABLE:
  ct:
    deposits_amount: "sum(ct.total_deposit_base)"
    deposits_amount_base: "sum(ct.total_deposit_base)"
    deposits_count: "sum(ct.deposit_count)"
    withdraw_amount: "sum(ct.total_withdraw_base)"
    withdraw_amount_base: "sum(ct.total_withdraw_base)"
    withdraw_count: "sum(ct.withdraw_count)"
    net_deposit: "sum(ct.total_deposit_base) - sum(ct.total_withdraw_base)"
    bet_amount: "sum(ct.total_bet_amount_base)"
    bet_amount_base: "sum(ct.total_bet_amount_base)"
    bet_count: "sum(ct.bet_count)"
    bonus_bet_amount: "sum(ct.total_bet_amount_bonus_base)"
    bonus_bet_count: "sum(ct.bet_bonus_count)"
    result_amount: "sum(ct.total_result_amount_base)"
    result_amount_base: "sum(ct.total_result_amount_base)"
    bonus_result_amount: "sum(ct.total_bonus_result_amount_base)"
    ggr: "sum(ct.total_bet_amount_base) - sum(ct.total_result_amount_base)"
    players_count: "uniqExact(ct.client_id)"
  cdt:
    deposits_amount: "sum(cdt.total_deposit_base)"
    deposits_amount_base: "sum(cdt.total_deposit_base)"
    deposits_count: "sum(cdt.deposit_count)"
    withdraw_amount: "sum(cdt.total_withdraw_base)"
    withdraw_amount_base: "sum(cdt.total_withdraw_base)"
    withdraw_count: "sum(cdt.withdraw_count)"
    net_deposit: "sum(cdt.total_deposit_base) - sum(cdt.total_withdraw_base)"
    bet_amount: "sum(cdt.total_bet_amount_base)"
    bet_amount_base: "sum(cdt.total_bet_amount_base)"
    bet_count: "sum(cdt.bet_count)"
    bonus_bet_amount: "sum(cdt.total_bet_amount_bonus_base)"
    bonus_bet_count: "sum(cdt.bet_bonus_count)"
    result_amount: "sum(cdt.total_result_amount_base)"
    result_amount_base: "sum(cdt.total_result_amount_base)"
    bonus_result_amount: "sum(cdt.total_bonus_result_amount_base)"
    ggr: "sum(cdt.total_bet_amount_base) - sum(cdt.total_result_amount_base)"
    players_count: "uniqExact(cdt.client_id)"
  cht:
    deposits_amount: "sum(cht.total_deposit_base)"
    deposits_amount_base: "sum(cht.total_deposit_base)"
    deposits_count: "sum(cht.deposit_count)"
    withdraw_amount: "sum(cht.total_withdraw_base)"
    withdraw_amount_base: "sum(cht.total_withdraw_base)"
    withdraw_count: "sum(cht.withdraw_count)"
    net_deposit: "sum(cht.total_deposit_base) - sum(cht.total_withdraw_base)"
    bet_amount: "sum(cht.total_bet_amount_base)"
    bet_amount_base: "sum(cht.total_bet_amount_base)"
    bet_count: "sum(cht.bet_count)"
    bonus_bet_amount: "sum(cht.total_bet_amount_bonus_base)"
    bonus_bet_count: "sum(cht.bet_bonus_count)"
    result_amount: "sum(cht.total_result_amount_base)"
    result_amount_base: "sum(cht.total_result_amount_base)"
    bonus_result_amount: "sum(cht.total_bonus_result_amount_base)"
    ggr: "sum(cht.total_bet_amount_base) - sum(cht.total_result_amount_base)"
    players_count: "uniqExact(cht.client_id)"
  cdbt:
    bet_amount: "sum(cdbt.total_bet_amount_base)"
    bet_amount_base: "sum(cdbt.total_bet_amount_base)"
    bet_count: "sum(cdbt.bet_count)"
    bonus_bet_amount: "sum(cdbt.total_bet_amount_bonus_base)"
    bonus_bet_count: "sum(cdbt.bet_bonus_count)"
    result_amount: "sum(cdbt.total_result_amount_base)"
    result_amount_base: "sum(cdbt.total_result_amount_base)"
    bonus_result_amount: "sum(cdbt.total_bonus_result_amount_base)"
    ggr: "sum(cdbt.total_bet_amount_base) - sum(cdbt.total_result_amount_base)"
    players_count: "uniqExact(cdbt.client_id)"
  chbt:
    bet_amount: "sum(chbt.total_bet_amount_base)"
    bet_amount_base: "sum(chbt.total_bet_amount_base)"
    bet_count: "sum(chbt.bet_count)"
    bonus_bet_amount: "sum(chbt.total_bet_amount_bonus_base)"
    bonus_bet_count: "sum(chbt.bet_bonus_count)"
    result_amount: "sum(chbt.total_result_amount_base)"
    result_amount_base: "sum(chbt.total_result_amount_base)"
    bonus_result_amount: "sum(chbt.total_bonus_result_amount_base)"
    ggr: "sum(chbt.total_bet_amount_base) - sum(chbt.total_result_amount_base)"
    players_count: "uniqExact(chbt.client_id)"
  cp:
    deposits_amount: "sum(cp.base_amount)"
    deposits_amount_base: "sum(cp.base_amount)"
    deposits_count: "count()"
    withdraw_amount: "sum(cp.base_amount)"
    withdraw_amount_base: "sum(cp.base_amount)"
    withdraw_count: "count()"
    depositing_players_count: "uniqExact(cp.client_id)"
    withdrawing_players_count: "uniqExact(cp.client_id)"
    players_count: "uniqExact(cp.client_id)"
    deposits_filters: ["cp.status = 5", "cp.payment_type = 'deposit'"]
    withdraws_filters: ["cp.status = 5", "cp.payment_type = 'payout'"]
  cb:
    bet_amount: "sum(cb.base_amount)"
    bet_amount_base: "sum(cb.base_amount)"
    bet_count: "count()"
    result_amount: "sum(cb.base_amount)"
    result_amount_base: "sum(cb.base_amount)"
    betting_players_count: "uniqExact(cb.client_id)"
    result_players_count: "uniqExact(cb.client_id)"
    players_count: "uniqExact(cb.client_id)"
    bets_filters: ["cb.operation = 'bet'"]
    results_filters: ["cb.operation = 'result'"]
  dc:
    ftd_count: "uniqExact(dc.client_id)"
    registrations_count: "uniqExact(dc.client_id)"
    players_count: "uniqExact(dc.client_id)"
    ftd_filters: ["dc.first_deposit_date IS NOT NULL"]
    registrations_filters: ["dc.registration_date IS NOT NULL"]
  cbon:
    bonus_amount: "sum(cbon.amount)"
    bonus_players_count: "uniqExact(cbon.client_id)"

OUTPUT_BINDING_RULES:
  - "For each routed base table, use only the exact player_list_by_base_table mapping for that same base table."
  - "Do not mix ct.client_id, cdt.client_id, cht.client_id, cdbt.client_id, chbt.client_id, cp.client_id, cb.client_id, or dc.client_id across routed contexts."
  - "If base table is cdt, use cdt.client_id. If base table is ct, use ct.client_id. Apply the same rule to all other base tables."

METRIC_OUTPUT_RULES:
  - "If requested-currency mode is active, SELECT must include the table currency column and metric expressions must come from CURRENCY_COLUMNS.original_metrics."
  - "For any leaderboard or top/bottom report, SELECT must include the metric used in ORDER BY."
  - "For any HAVING comparison, SELECT must include all compared metrics."
  - "For players whose withdrawals are greater than deposits, include withdraw_amount, deposits_amount, and withdrawal_minus_deposit."
  - "For players who made bets, include bet_amount unless the user explicitly asks only for a list/count."
  - "For players who made withdrawals or payout, include withdraw_amount unless the user explicitly asks only for a list/count."
  - "For players who made deposits, include deposits_amount unless the user explicitly asks only for a list/count."
  - "For top bonus wins or bonus results, include bonus_result_amount."
  - "For claimed bonus only, player list may include only client_id and username unless amount/type/status is requested."
  - "Do not order by a metric that is not present in SELECT."

OUTPUT_RULES:
  player_list_by_base_table:
    ct: ["ct.client_id", "cs.username"]
    cdt: ["cdt.client_id", "cs.username"]
    cht: ["cht.client_id", "cs.username"]
    cdbt: ["cdbt.client_id", "cs.username"]
    chbt: ["chbt.client_id", "cs.username"]
    cp: ["cp.client_id", "cs.username"]
    cb: ["cb.client_id", "cs.username"]
    dc: ["dc.client_id", "cs.username"]
    cbon: ["cbon.client_id", "cs.username"]
    ca: ["ca.client_id", "cs.username"]
    ce: ["ce.client_id", "cs.username"]
    cse: ["cse.client_id", "cs.username"]
  ftd_player_list:
    required_columns: ["dc.client_id", "cs.username", "dc.first_deposit_date"]
  registration_player_list:
    required_columns: ["dc.client_id", "cs.username", "dc.registration_date"]
  bonus_player_list:
    required_columns: ["cbon.client_id", "cs.username"]

GROUPING_AND_CURRENCY_RULES:
  - "If base table grain includes currency and user did not request currency, aggregate over currency before final output."
  - "If user explicitly requests currency split, keep currency in GROUP BY and output."
  - "For top players from ct/cdt/cht/cdbt/chbt, group by the explicit player_list_by_base_table columns first, then order by metric, then limit."
  - "For country, game, or provider leaderboards, group by requested dimension first, then order by metric, then limit."


CLICKHOUSE_SQL_SAFETY_RULES:
  - "Use ClickHouse LIMIT syntax: LIMIT N. Do not use LIMIT offset, count format."
  - "Never use toTime() with string literals."
  - "For time-of-day filtering on DateTime columns, use toHour(column) or full DateTime boundaries."

DATE_PRESETS:
  today:
    date_start: "today()"
    date_end: "today() + 1"
  yesterday:
    date_start: "today() - 1"
    date_end: "today()"
  last_7_days:
    date_start: "today() - 6"
    date_end: "today() + 1"
  last_30_days:
    date_start: "today() - 29"
    date_end: "today() + 1"
  this_month:
    date_start: "toStartOfMonth(today())"
    date_end: "addMonths(toStartOfMonth(today()), 1)"
  last_month:
    date_start: "addMonths(toStartOfMonth(today()), -1)"
    date_end: "toStartOfMonth(today())"


BONUS_RULES:
  claimed_bonus:
    conditions:
      - "cbon.created_at in requested period"
  active_bonus:
    conditions:
      - "cbon.status = 'activated'"
  rollovered_bonus:
    conditions:
      - "cbon.status = 'rollovered'"
  claimed_cash_bonus:
    conditions:
      - "cbon.created_at in requested period"
      - "cbon.amount > 0"
  claimed_freespin_bonus:
    conditions:
      - "cbon.created_at in requested period"
      - "cbon.amount = 0"

CURRENCY_MODE:
  default_mode: base
  requested_currency_mode: supported_without_fx_conversion
  phase: 2_ready
  rules:
    - "Use *_base and base_amount columns for all monetary outputs by default."
    - "If the prompt explicitly requests currency, original currency, by-currency output, a specific currency code, or client-currency output, use original amount columns."
    - "If requested-currency mode is active, include currency in SELECT and GROUP BY for aggregated reports unless filtering to one explicit currency."
    - "If the prompt requests a specific currency code, filter by currency using lowercase code where a currency column exists."
    - "Do not perform FX conversion in SQL."
    - "If requested currency requires conversion and no stored original-currency amount exists, return SELECT 'UNSUPPORTED_REQUEST' AS error."

PROMPT_OUTPUT_HINTS:
  if_prompt_mentions_currency:
    currency_mode: requested_currency
    include_dimension: currency
    use_original_amount_columns: true
    group_by_currency_when_aggregated: true
  if_prompt_mentions_specific_currency_code:
    currency_mode: requested_currency
    include_dimension: currency
    use_original_amount_columns: true
    filter_currency_to_requested_code: true
  if_prompt_mentions_client_currency:
    currency_mode: requested_currency
    include_dimension: currency
    use_original_amount_columns: true
    group_by_currency_when_aggregated: true
  otherwise:
    currency_mode: base
    use_base_amount_columns: true

RANKING_DEFAULTS:
  deposit:
    default_rank_metric: deposits_amount
  withdraw:
    default_rank_metric: withdraw_amount
  bet:
    default_rank_metric: bet_amount
  win:
    default_rank_metric: result_amount
  result:
    default_rank_metric: result_amount
  bonus_win:
    default_rank_metric: bonus_result_amount
  bonus_result:
    default_rank_metric: bonus_result_amount

DEFAULT_INTERPRETATIONS:
  top_players:
    default_metric: bet_amount
    default_output: player_list
    default_query_class: leaderboard
    routing_preference:
      all_time: ct
      date_range_or_daily: chbt
      hourly_or_intraday: chbt
  players:
    if_prompt_has_count_synonym: players_count
    else: player_list
  ftd:
    if_prompt_has_count_synonym: ftd_count
    else: ftd_player_list
  bonus_players:
    if_prompt_has_count_synonym: bonus_players_count
    else: bonus_player_list
  claimed_bonus:
    default_query_class: bonus_query

COUNT_SYNONYMS:
  - count
  - number
  - how many
  - qty
  - quantity
  - total number


SUPPORTED_CURRENCY_CODES:
  - eur
  - usd
  - amd
  - rub
  - usdt
  - btc
  - eth

ENUM_VALUES:
  payment_type:
    deposit: "deposit"
    deposite: "deposit"
    deposited: "deposit"
    withdraw: "payout"
    payout: "payout"
    cashout: "payout"
  operation:
    bet: "bet"
    result: "result"

SEMANTIC_ALIASES:
  deposit: [deposit, deposits, deposited]
  withdrawal: [withdraw, withdrawal, withdrawals, cashout, payout]
  deposit_payment_type: [deposit, deposits, deposited, deposite]
  withdraw_payment_type: [withdraw, withdrawal, withdrawals, cashout, payout]
  player: [player, players, client, clients, user, users]
  registration: [registration, registrations, registered players, signups, sign ups]
  bet_amount: [bet amount, total bets, wager amount, wagered, stake amount]
  result_amount: [result amount, win amount, payout amount, returned amount]
  ggr: [ggr, gross gaming revenue]
  ftd: [ftd, first deposit, first time deposit, first-time deposit]
  claimed_bonus: [claimed bonus, claimed bonuses, bonus claimed]
  active_bonus: [active bonus, active bonuses]
  rollovered_bonus: [rollovered bonus, rollovered bonuses, finished bonus, finished bonuses]
  claimed_cash_bonus: [claimed cash bonus, claimed cash bonuses, cash bonus]
  claimed_freespin_bonus: [claimed freespin bonus, claimed freespin bonuses, freespin bonus, freespin bonuses]
  bonus: [bonus, bonuses, claimed bonus, received bonus]


COMPOSITE_ANCHOR_RULES:
  - "If prompt contains multiple conditions with 'and' and also contains ranking intent (top), select base table from the condition that provides a measurable amount metric."
  - "Withdraw → use client_payments as base"
  - "Deposit → use client_payments as base"
  - "Bet/Win → use betting tables"
  - "Bonus wins → use betting aggregate tables"
  - "Other conditions must be applied as filters (subquery or join)"

COMPOSITE_RANKING_DEFAULTS:
  withdraw:
    metric: withdraw_amount
    expression: sum(cp.base_amount)
  deposit:
    metric: deposits_amount
    expression: sum(cp.base_amount)

COMPOSITE_PROMPT_RULES:
  - "For prompts like 'claimed bonus yesterday and made withdraw', keep both the claimed-bonus condition and the withdrawal condition."
  - "For composite ranked prompts involving withdraw or deposit without count wording, rank by monetary amount by default."
  - "For composite prompts involving made bets, made withdraw/payout, made deposit, wins, or bonus wins, include the corresponding amount metric in SELECT."
  - "For non-ranked composite prompts, include the relevant event amount metric when one condition is monetary and the user did not explicitly request list-only output."

OUTPUT_EXAMPLES:
  - prompt: "players whose withdrawals are greater than deposits"
    required_select: ["client_id", "username", "withdraw_amount", "deposits_amount", "withdrawal_minus_deposit"]
  - prompt: "players who claimed bonus and made bets yesterday"
    required_select: ["client_id", "username", "bet_amount"]
  - prompt: "players who received bonus and made payout"
    required_select: ["client_id", "username", "withdraw_amount"]
  - prompt: "top bonus wins yesterday"
    required_select: ["client_id", "username", "bonus_result_amount"]

UNSUPPORTED_OR_BLOCKED:
  - "Do not infer refund semantics beyond approved bet/result operation values."
  - "Do not infer non-success payment statuses beyond success = 5."
  - "Do not use legacy MySQL epoch timestamps as primary reporting time fields when a native reporting table exists."
  - "Do not create cross-fact metrics unless explicitly defined."
  - "Do not answer FTD amount without an approved derivation rule."
`
