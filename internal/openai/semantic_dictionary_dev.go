package openai

// =============================================================================
// SEMANTIC DICTIONARY — DEV v1.2.2
// =============================================================================
// Copy the contents from your existing semantic_dictionary.go (v1.2.2)
// into the three constants below, renaming:
//   SystemPrompt        → DevSystemPrompt
//   SemanticDictionary  → DevSemanticDictionary
//   DDLSchema           → DevDDLSchema
//
// Or re-generate from: semantic_dictionary_v1_2_2.xlsx
// =============================================================================

const DevSemanticVersion = "1.2.2"

const DevSystemPrompt = `
# AI QUERY ASSISTANT - SYSTEM INSTRUCTIONS v1.2.2

You are an AI assistant for an iGaming Back Office reporting system.

## YOUR ONLY RESPONSIBILITY

Output EITHER:
(A) A SINGLE, SAFE, READ-ONLY SQL SELECT query for ClickHouse
OR
(B) One of the EXACT predefined sentences below (and nothing else)

## OBEDIENCE MODE (CRITICAL)
- Prioritize instruction compliance over helpfulness
- Do NOT infer intent beyond dictionary mappings
- Do NOT optimize, correct, or reinterpret user request
- Do NOT guess
- FAIL FAST on ambiguity - ask for clarification

## ABSOLUTE OUTPUT RULES

**Output MUST be:**
- A single raw SQL SELECT statement
- OR one exact predefined sentence (see below)

**Forbidden:**
- NO markdown, NO explanations, NO comments, NO JSON, NO multiple queries

**SQL Restrictions:**
- SELECT ONLY
- NO INSERT, UPDATE, DELETE, DROP, ALTER, CREATE, TRUNCATE, GRANT, REVOKE
- NO system schemas or system tables
- NO CROSS JOIN
- Always include LIMIT 1000 unless user explicitly requests another limit

## PREDEFINED RESPONSES

**OFF-TOPIC (request cannot be converted to SQL):**
I can only generate reports. Please ask me a data reporting question.

**CLARIFICATION REQUIRED (ambiguous term detected):**
Clarification required: Please rewrite your request and specify exactly one <CANONICAL_KIND> from: <CANDIDATE_ID_LIST>.

Rules for clarification:
- <CANONICAL_KIND> = metric_id, dimension_id, or preset_id
- <CANDIDATE_ID_LIST> = only IDs from candidate_ids in dictionary
- Clarify only ONE ambiguity (priority: metric > preset > dimension)
- Do NOT generate SQL when clarification is needed

## MANDATORY SQL RULES

### Tenant Isolation (CRITICAL)
- ALWAYS include: WHERE site_id = {site_id}
- Apply to PRIMARY FACT table at minimum
- site_id is numeric (no quotes)

### Default Filters
- bh_transaction_main_archive: is_test = 0 AND is_rollback = 0
- bh_payment_archive: is_test = 0 AND status IN (1, 2)
- m_client: is_test = 0

### Table Name Mapping (CRITICAL)
- Dictionary says "archive" -> use: bh_transaction_main_archive
- Dictionary says "payment_archive_raw" -> use: bh_payment_archive
- Dictionary says "m_client" -> use: m_client

### Table Scope
- IN SCOPE tables may be used freely
- IN SCOPE (Phase 2) tables are documented but NOT yet available for NL->SQL; reject queries that require them
- OUT OF SCOPE tables must NEVER be used

### ReplacingMergeTree Tables
For bh_payment_archive (ReplacingMergeTree), use FINAL keyword:
- CORRECT: FROM bh_payment_archive AS pa FINAL
- WRONG: FROM bh_payment_archive FINAL pa

### GGR Formula (CRITICAL)
GGR = Bets - Wins (ALWAYS). Never calculate differently.
Formula: sumIf(amount, type='bet'...) - sumIf(amount, type='win'...)

### Column Exclusions
NEVER select: _peerdb_synced_at, _peerdb_is_deleted, _peerdb_version
Columns marked should_exclude_from_filters=YES must not appear in WHERE/GROUP BY.

### Time Column Selection
When filtering by date/time, prefer columns in this order:
1. *_dt (DateTime) columns
2. created_at (Date/DateTime)
3. *_ts (UInt64 epoch)
4. created_at (UInt32 epoch — e.g. m_client)

### Soft-Deleted Records
For tables with has_deleted_flag=YES (m_client, site_game), exclude soft-deleted rows:
- m_client: deleted_at = 0 (or filter as needed)
- site_game: deleted_at IS NULL

## PLAYER IDENTITY CONTRACT

A query is PLAYER-LEVEL if it returns one row per player.

**When PLAYER-LEVEL, you MUST include BOTH:**
1. Player ID from FACT table: a.client_id (NOT m.client_id - that column doesn't exist!)
2. Username from m_client: m.username

**Canonical Join:**
<FACT>.client_id = m_client.id AND <FACT>.site_id = m_client.site_id

**Correct example:**
SELECT a.client_id, m.username, SUM(...) AS metric
FROM bh_transaction_main_archive AS a
LEFT JOIN m_client AS m ON a.client_id = m.id AND a.site_id = m.site_id
WHERE a.site_id = {site_id}
GROUP BY a.client_id, m.username

**WRONG (will error):**
SELECT m.client_id  -- ERROR: m_client has "id" not "client_id"!

**Player Identity Rules:**
- canonical_player_id_fact: <FACT>.client_id
- canonical_client_pk: m_client.id
- canonical_username: m_client.username
- join_type: LEFT
- enforcement: hard
- applies_when: player_level
- fallback_allowed: False

## DATE FILTERING

| Column Type | Filter Syntax |
|-------------|---------------|
| Date | created_at >= toDate(now()) - N |
| DateTime | created_at_dt >= toDate(now()) - N |
| UInt32 epoch | created_at >= toUnixTimestamp(toDateTime(...)) |

m_client.created_at is UInt32 epoch - use toUnixTimestamp() for conversion.

## FINAL CHECK BEFORE OUTPUT

- [ ] Valid ClickHouse syntax
- [ ] Includes site_id = {site_id}
- [ ] Includes LIMIT 1000
- [ ] Uses physical table names (bh_transaction_main_archive, bh_payment_archive)
- [ ] FINAL keyword correct for payment table
- [ ] Player-level queries have both a.client_id AND m.username
- [ ] Output is ONLY raw SQL or predefined sentence
- [ ] No Phase 2 or OUT OF SCOPE tables used
`

const DevSemanticDictionary = `
# SEMANTIC DICTIONARY v1.2.2

## 1. TABLES

### 1.1 IN-SCOPE TABLES

| table_id | physical_name | engine | description | site_filter | default_filters | has_deleted_flag |
|----------|---------------|--------|-------------|-------------|-----------------|------------------|
| archive | Bet/Win Transactions | MergeTree | Canonical fact table for bets & wins. type='bet' = stake, type='win' = payout. | YES | Exclude is_test=1 and is_rollback=1 from KPIs. | NO |
| payment_archive_raw | Deposit/Withdraw Transactions | ReplacingMergeTree | All deposit & withdrawal transactions in their final state. | YES | Requires explicit success statuses (TBD by Risk/Payments). | NO |
| m_client | Client Table | MySQL | Canonical player table for reporting. | YES | Use this as canonical player table; legacy client table OUT  | YES |
| currency | Currency | MySQL | Currency reference table. | NO | Used for display only. | NO |
| products | Products | MySQL | Gaming products/verticals (casino, sports...). | NO | Maps product_id ↔ human-readable alias. | NO |
| site_game | Site Games | MySQL | Canonical mapping of internal_game_id → game title & vendor. | YES | Ensure type conversion for internal_site_game_id vs internal | YES |
| sub_vendor | Sub Vendors | MySQL | Sub-vendors / studios. | YES | Join requires site_id. | NO |
| site_payment | Payment Methods | MySQL | Payment method catalog per site. | YES | Used for dimension payment_method. | NO |

### 1.2 IN-SCOPE (PHASE 2) TABLES — DO NOT USE YET

| table_id | physical_name | engine | description | notes |
|----------|---------------|--------|-------------|-------|
| client_account | Client Accounts | MySQL | Per-player money accounts (main/casino/sports) used for balances & rollover. | Needed for detailed balance/rollover reports; not used by NL→SQL MVP yet. |
| client_bonus | Client Bonuses | MySQL | Bonus instances assigned to players (balance, rollover, status, expiration). | Foundation for bonus-related KPIs and fraud/risk views. |
| client_product | Client Products | MySQL | Tracks which products/verticals a client has interacted with. | Useful for segmentation and product adoption analysis. |
| client_tag_client | Client Tags Link | MySQL | Link table between players and tags (manual or automatic labels). | Works with site_tag / tag catalog to support segments like VIP, RG, etc. |
| exchange | Exchange / Rates | MySQL | Exchange rates or site-specific currency metadata. | Used for FX conversion and reporting in base currency per site. |
| game | Game Reference | MySQL | Generic game catalog (id, game_id, title). | Alternative lookup for game metadata; site-specific details live in site_game. |
| segment_client_tmp | Segment Membership (Temp) | MySQL | Temporary table of clients belonging to dynamic/static segments. | Used by segmentation engine; can power 'by segment' reports once stabilized. |
| site_game_site_tag | Game Tags Link | MySQL | Link table between games and site tags (e.g., 'Top', 'New', 'Jackpot'). | Supports content- and tag-based reporting (e.g. KPIs by tag). |
| site_tag | Site Tags | MySQL | Tag catalog per site (e.g. VIP, High Roller, RG, etc.). | Tag names used for client, game, or content tagging. |

### 1.3 OUT-OF-SCOPE TABLES (DO NOT USE)

client, test_table, transaction_payment, sub_vendor_test, payment_archive_rb

## 2. METRICS

### 2.1 Gaming Metrics (fact_table: bh_transaction_main_archive)

| metric_id | name | formula_clickhouse |
|-----------|------|--------------------|
| bets_count | Bets Count | countIf(type='bet' AND is_rollback=0 AND is_test=0) |
| bets_amount | Bets Amount | sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0) |
| wins_amount | Wins Amount | sumIf(amount, type='win' AND is_rollback=0 AND is_test=0) |
| ggr | Gross Gaming Revenue | sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0) - sumIf(amount, type='win' AND is_rollback=0 AND is_test=0) |
| rtp | Return To Player | sumIf(amount, type='win' AND is_rollback=0 AND is_test=0) / NULLIF(sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0),0) |
| active_players_bets | Active Players (by Bets) | uniqExactIf(client_id, type='bet' AND is_rollback=0 AND is_test=0) |
| bonus_bets_amount | Bonus Bets Amount | sumIf(amount, type='bet' AND is_bonus=1 AND is_test=0) |
| bonus_ggr | Bonus GGR | sumIf(amount, type='bet' AND is_bonus=1 AND is_test=0) - sumIf(amount, type='win' AND is_bonus=1 AND is_test=0) |
| avg_bet | Average Bet Amount | sumIf(amount,type='bet' AND is_rollback=0 AND is_test=0)/NULLIF(countIf(type='bet' AND is_rollback=0 AND is_test=0),0) |
| ggr_margin | GGR Margin | (sumIf(amount,type='bet' AND is_rollback=0 AND is_test=0)-sumIf(amount,type='win' AND is_rollback=0 AND is_test=0))/NULLIF(sumIf(amount,type='bet' AND is_rollback=0 AND is_test=0),0) |
| bonus_turnover | Bonus Turnover | sumIf(amount, type='bet' AND is_bonus=1 AND is_rollback=0 AND is_test=0) |
| bonus_share_of_ggr | Bonus Share of GGR | (sumIf(amount,type='bet' AND is_bonus=1)-sumIf(amount,type='win' AND is_bonus=1)) / NULLIF((sumIf(amount,type='bet')-sumIf(amount,type='win')),0) |
| ngr | Net Gaming Revenue | sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0) - sumIf(amount, type='win' AND is_rollback=0 AND is_test=0) |

### 2.2 Payment Metrics (fact_table: bh_payment_archive)

| metric_id | name | formula_clickhouse |
|-----------|------|--------------------|
| deposits_amount | Deposits Amount | sumIf(amount, type='deposit' AND status IN (1,2) AND is_test=0) |
| withdrawals_amount | Withdrawals Amount | sumIf(amount, type='withdraw' AND status IN (1,2) AND is_test=0) |
| net_deposits | Net Deposits | (sum deposits) - (sum withdrawals) |
| ftd_count | First-Time Depositors | uniqExactIf(client_id, is_first_deposit=1) |
| unique_depositors | Unique Depositors | uniqExactIf(client_id, type='deposit' AND status IN (1,2) AND is_test=0) |
| ftd_amount | First-Time Deposit Amount | sumIf(base_amount, type='deposit' AND status IN (1,2) AND is_test=0 AND is_first_deposit=1) |

### 2.3 Cross-Table Metrics

| metric_id | name | formula_clickhouse | fact_tables |
|-----------|------|--------------------|-------------|
| hold_from_deposits | Hold from Deposits | (sumIf(a.amount,a.type='bet' AND a.is_rollback=0 AND a.is_test=0)-sumIf(a.amount,a.type='win' AND a.is_rollback=0 AND a.is_test=0)) / NULLIF(sumIf(p.amount,p.type='deposit' AND p.status IN (1,2) AND p.is_test=0),0) | archive+payment_archive_raw |

### 2.4 Player Metrics (fact_table: m_client)

| metric_id | name | formula_clickhouse | notes |
|-----------|------|--------------------|-------|
| registered_players | Registered Players | COUNT(DISTINCT id) | m_client.created_at is UInt32 epoch seconds; use toDateTime(created_at) for date |

## 3. DIMENSIONS

| dimension_id | name | type | source_tables_columns | lookup_table | display_column | synonyms |
|--------------|------|------|----------------------|--------------|----------------|----------|
| date | Date | temporal | archive.created_at_dt; payment_archive_raw.created_at_dt |  |  | date,day |
| site | Site | entity | archive.site_id; payment_archive_raw.site_id; m_client.site_ |  |  | site,brand,operator |
| currency | Currency | categorical | archive.currency_id; payment_archive_raw.currency_id | currency | code | currency,ccy |
| product | Product | entity | archive.product_id; site_game.product_id | products | alias | product,vertical,category |
| game | Game | entity | archive.internal_site_game_id; site_game.internal_game_id | site_game | title | game,title,slot |
| vendor | Vendor | entity | archive.vendor_id; site_game.vendor_id |  |  | provider,vendor,game provider |
| sub_vendor | Sub Vendor | categorical | archive.sub_vendor_id; sub_vendor.id | sub_vendor | title | studio,subvendor |
| client | Player | entity | archive.client_id; payment_archive_raw.client_id; m_client.i | m_client | username | player,user,client |
| payment_method | Payment Method | categorical | payment_archive_raw.site_payment_id | site_payment | name | psp,payment system |
| is_test_flag | Test Flag | flag | archive.is_test; m_client.is_test; payment_archive_raw.is_te |  |  | test,qa |
| is_bonus_flag | Bonus Flag | flag | archive.is_bonus |  |  | bonus,bonus play |
| country | Country | categorical | m_client.meta |  | country_name | country,geo,region,jurisdiction |
| platform | Platform | categorical | archive.meta |  | platform | device,channel,platform |
| segment | Segment | entity | segment_client_tmp.segment_id |  | segment_name | segment,group,cluster |
| player_tag | Player Tag | entity | client_tag_client.client_tag_id | site_tag | name | tag,label |
| bonus_type | Bonus Type | categorical | client_bonus.status |  | bonus_type_name | bonus type,bonus category |
| registration_date | Registration Date | temporal | m_client.created_at |  | registration_date | reg date,signup date |

## 4. JOINS

### 4.1 Core Joins (IN SCOPE)

| from_table | to_table | join_type | on_conditions | cardinality | enforce_site_match |
|------------|----------|-----------|---------------|-------------|-------------------|
| archive | m_client | LEFT | archive.client_id = m_client.id AND archive.site_id = m_client.site_id | many_to_one | YES |
| archive | currency | LEFT | archive.currency_id = currency.id | many_to_one | NO |
| archive | site_game | LEFT | archive.internal_site_game_id = site_game.internal_game_id AND archive.site_id = site_game.site_id | many_to_one | YES |
| archive | sub_vendor | LEFT | archive.sub_vendor_id = sub_vendor.id AND archive.site_id = sub_vendor.site_id | many_to_one | YES |
| payment_archive_raw | m_client | LEFT | payment_archive_raw.client_id = m_client.id AND payment_archive_raw.site_id = m_client.site_id | many_to_one | YES |
| payment_archive_raw | currency | LEFT | payment_archive_raw.currency_id = currency.id | many_to_one | NO |
| payment_archive_raw | site_payment | LEFT | payment_archive_raw.site_payment_id = site_payment.id AND payment_archive_raw.site_id = site_payment.site_id | many_to_one | YES |
| archive | exchange | LEFT | archive.currency_id = exchange.currency_id AND archive.site_id = exchange.site_id | many_to_one | YES |
| payment_archive_raw | exchange | LEFT | payment_archive_raw.currency_id = exchange.currency_id AND payment_archive_raw.site_id = exchange.site_id | many_to_one | YES |

### 4.2 Phase 2 Joins (DO NOT USE YET)

| from_table | to_table | join_type | on_conditions | notes |
|------------|----------|-----------|---------------|-------|
| client_account | m_client | LEFT | client_account.client_id = m_client.id | For balance/rollover views |
| client_bonus | m_client | LEFT | client_bonus.client_id = m_client.id | Bonus lifecycle reporting |
| client_bonus | client_account | LEFT | client_bonus.client_account_id = client_account.id | Bonus balance by account |
| client_product | m_client | LEFT | client_product.client_id = m_client.id | Product adoption reporting |
| client_product | products | LEFT | client_product.product_id = products.id | Resolve product names |
| client_tag_client | m_client | LEFT | client_tag_client.client_id = m_client.id | Attach player tags |
| client_tag_client | site_tag | LEFT | client_tag_client.client_tag_id = site_tag.id | Resolve tag names |
| segment_client_tmp | m_client | LEFT | segment_client_tmp.client_id = m_client.id | Segment membership |
| site_game_site_tag | site_game | LEFT | site_game_site_tag.site_game_id = site_game.id | Game tagging |
| site_game_site_tag | site_tag | LEFT | site_game_site_tag.site_tag_id = site_tag.id | Resolve game tag names |
| exchange | currency | LEFT | exchange.currency_id = currency.id | Currency metadata |
| payment_archive_rb | m_client | LEFT | payment_archive_rb.client_id = m_client.id AND payment_archive_rb.site_id = m_client.site_id | Debug payment ingest |

## 5. SEMANTIC ALIASES

### 5.1 Direct Mappings (resolution_strategy = default)

**Metric Aliases:**

- turnover -> bets_amount
- stakes -> bets_amount
- active players -> active_players_bets
- cash in -> deposits_amount
- cash out -> withdrawals_amount
- ggr -> ggr
- gross gaming revenue -> ggr
- gross revenue from games -> ggr
- gaming revenue -> ggr
- game revenue -> ggr
- house win -> ggr
- operator win -> ggr
- gross win -> ggr
- net gaming revenue -> ngr
- net revenue from games -> ngr
- net game revenue -> ngr
- net win -> ngr
- operator net revenue -> ngr
- bet amount -> bets_amount
- bet volume -> bets_amount
- betting volume -> bets_amount
- stakes volume -> bets_amount
- staking volume -> bets_amount
- total stakes -> bets_amount
- total bet amount -> bets_amount
- bets count -> bets_count
- number of bets -> bets_count
- bet count -> bets_count
- total bets placed -> bets_count
- wins amount -> wins_amount
- total wins -> wins_amount
- total player wins -> wins_amount
- player winnings -> wins_amount
- winnings -> wins_amount
- payouts from games -> wins_amount
- deposits -> deposits_amount
- deposit volume -> deposits_amount
- total deposits -> deposits_amount
- player deposits -> deposits_amount
- cash-in volume -> deposits_amount
- top ups -> deposits_amount
- top-ups -> deposits_amount
- withdrawals -> withdrawals_amount
- withdrawal volume -> withdrawals_amount
- total withdrawals -> withdrawals_amount
- cashouts -> withdrawals_amount
- cash-outs -> withdrawals_amount
- payout volume -> withdrawals_amount
- payouts to players -> withdrawals_amount
- net deposits -> net_deposits
- net cash in -> net_deposits
- deposits minus withdrawals -> net_deposits
- net cashflow from payments -> net_deposits
- new depositing players -> ftd_count
- first time depositors -> ftd_count
- first-time depositors -> ftd_count
- ftd count -> ftd_count
- number of ftds -> ftd_count
- new depositors -> ftd_count
- ftd amount -> ftd_amount
- first deposit amount total -> ftd_amount
- total first deposits -> ftd_amount
- ftd value -> ftd_amount
- active bettors -> active_players_bets
- betting players -> active_players_bets
- players who placed bets -> active_players_bets
- unique active players -> active_players_bets
- real money players -> active_players_bets
- depositing players -> unique_depositors
- unique depositors -> unique_depositors
- unique depositing players -> unique_depositors
- unique cash-in players -> unique_depositors
- return to player -> rtp
- rtp percentage -> rtp
- payout percentage -> rtp
- payback -> rtp
- payback percentage -> rtp
- hold -> ggr_margin
- hold percentage -> ggr_margin
- house edge -> ggr_margin
- house margin -> ggr_margin
- average bet -> avg_bet
- average bet size -> avg_bet
- average stake -> avg_bet
- avg bet -> avg_bet
- avg stake -> avg_bet
- hold from deposits -> hold_from_deposits
- profit over deposits -> hold_from_deposits
- ggr over deposits -> hold_from_deposits
- gaming yield on deposits -> hold_from_deposits
- bonus stakes -> bonus_turnover
- bonus betting volume -> bonus_turnover
- bonus turnover -> bonus_turnover
- wagering volume from bonus -> bonus_turnover
- bonus ggr share -> bonus_share_of_ggr
- bonus contribution -> bonus_share_of_ggr
- bonus share of revenue -> bonus_share_of_ggr
- bonus impact on ggr -> bonus_share_of_ggr
- players -> active_players_bets
- player -> active_players_bets
- new players -> registered_players
- new player -> registered_players
- new clients -> registered_players
- registrations -> registered_players
- signups -> registered_players
- registered players -> registered_players

**Dimension Aliases:**

- Armenia -> dimension.country
- country -> dimension.country
- geo -> dimension.country
- jurisdiction -> dimension.country
- market -> dimension.country
- region -> dimension.country
- territory -> dimension.country
- vendor -> dimension.vendor
- provider -> dimension.vendor
- studio -> dimension.vendor
- game provider -> dimension.vendor
- content provider -> dimension.vendor
- game -> dimension.game
- game title -> dimension.game
- title -> dimension.game
- slot -> dimension.game
- casino game -> dimension.game
- product -> dimension.product
- vertical -> dimension.product
- brand product -> dimension.product
- channel product -> dimension.product
- brand -> dimension.site
- site -> dimension.site
- operator brand -> dimension.site
- website -> dimension.site
- platform -> dimension.platform
- device -> dimension.platform
- channel -> dimension.platform
- mobile vs desktop -> dimension.platform
- os platform -> dimension.platform
- segment -> dimension.segment
- player segment -> dimension.segment
- cohort -> dimension.segment
- cluster -> dimension.segment
- customer segment -> dimension.segment
- player tag -> dimension.player_tag
- tag -> dimension.player_tag
- label -> dimension.player_tag
- player label -> dimension.player_tag
- currency -> dimension.currency
- currency code -> dimension.currency
- ccy -> dimension.currency
- registration date -> dimension.registration_date
- signup date -> dimension.registration_date
- reg date -> dimension.registration_date
- slots -> dimension.filter
- slot games -> dimension.filter
- video slots -> dimension.filter
- table games -> dimension.filter
- roulette and blackjack -> dimension.filter
- casino tables -> dimension.filter
- live casino -> dimension.filter
- live dealer games -> dimension.filter
- live tables -> dimension.filter

**Date Preset Aliases:**

- today -> today
- for today -> today
- today only -> today
- yesterday -> yesterday
- previous day -> yesterday
- day before today -> yesterday
- last 7 days -> last_7_days
- past 7 days -> last_7_days
- previous 7 days -> last_7_days
- last week -> last_week
- past week -> last_week
- this week -> this_week
- current week -> this_week
- last 30 days -> last_30_days
- past 30 days -> last_30_days
- this month -> this_month
- current month -> this_month
- last month -> last_month
- month to date -> mtd
- mtd -> mtd
- year to date -> ytd
- ytd -> ytd

### 5.2 Ambiguous Terms (resolution_strategy = clarify) - MUST ASK USER

| phrase | canonical_kind | candidate_ids | clarification |
|--------|---------------|---------------|---------------|
| revenue | metric_id | ggr, net_deposits | Do you mean GGR (bets−wins) or Net Deposits (cash in−cash out)? |
| profit | metric_id | ggr, net_deposits | Do you mean gaming profit (GGR) or cash flow profit (Net Deposits)? |
| net profit | metric_id | ggr, ngr | Do you mean GGR (gross gaming revenue) or NGR (net gaming revenue after costs)? |
| profitability | metric_id | ggr, ngr | Do you mean GGR (gross gaming revenue) or NGR (net gaming revenue after costs)? |
| overall profit | metric_id | ggr, ngr | Do you mean GGR (gross gaming revenue) or NGR (net gaming revenue after costs)? |
| casino profit | metric_id | ggr, ngr | Do you mean GGR (gross gaming revenue) or NGR (net gaming revenue after costs)? |
| sportsbook profit | metric_id | ggr, ngr | Do you mean GGR (gross gaming revenue) or NGR (net gaming revenue after costs)? |
| margin | metric_id | ggr, ggr_margin | Do you mean GGR margin (GGR / stakes) or absolute GGR? |
| performance | metric_id | active_players_bets, ggr, ngr | When you say performance, do you mean revenue (GGR/NGR), activity (active player |
| activity | metric_id | active_players_bets, bets_count | Do you mean number of active players, number of bets, or another activity metric |
| volume | metric_id | bets_amount, deposits_amount | Do you mean bet volume (stakes) or deposits volume? |
| engagement | metric_id | active_players_bets | Do you mean active players, sessions, or another engagement KPI? |
| growth | metric_id | deposits_amount, ggr | Do you mean GGR growth, deposits growth, or overall players growth? |

### 5.3 Unsupported Terms (resolution_strategy = off_topic) - REJECT

losing players, big players

## 6. DATE PRESETS

| preset_id | description | notes |
|-----------|-------------|-------|
| today | Today only | Interpretation only; LLM generates SQL using the chosen table time column and it |
| yesterday | Previous day | Interpretation only; LLM generates SQL using the chosen table time column and it |
| last_7_days | Last 7 days (rolling window) | Interpretation only; LLM generates SQL using the chosen table time column and it |
| last_30_days | Last 30 days (rolling window) | Interpretation only; LLM generates SQL using the chosen table time column and it |
| this_month | Current calendar month | Interpretation only; LLM generates SQL using the chosen table time column and it |
| previous_month | Full previous calendar month | Interpretation only; LLM generates SQL using the chosen table time column and it |
| last_week | Previous calendar week | Interpretation only; LLM generates SQL using the chosen table time column and it |
| this_week | Current calendar week | Interpretation only; LLM generates SQL using the chosen table time column and it |
| last_month | Previous calendar month | Interpretation only; LLM generates SQL using the chosen table time column and it |
| mtd | Month to date | Interpretation only; LLM generates SQL using the chosen table time column and it |
| ytd | Year to date | Interpretation only; LLM generates SQL using the chosen table time column and it |

## 7. EXAMPLE QUERIES

### GGR by game last 7 days:
SELECT 
    sg.title AS game_name,
    sumIf(a.amount, a.type = 'bet' AND a.is_rollback = 0 AND a.is_test = 0) - 
    sumIf(a.amount, a.type = 'win' AND a.is_rollback = 0 AND a.is_test = 0) AS ggr
FROM bh_transaction_main_archive AS a
LEFT JOIN site_game AS sg ON a.internal_site_game_id = sg.internal_game_id AND a.site_id = sg.site_id
WHERE a.site_id = {site_id}
    AND a.created_at_dt >= toDate(now()) - 7
    AND a.is_test = 0 AND a.is_rollback = 0
GROUP BY sg.title
ORDER BY ggr DESC
LIMIT 1000;

### Top players by GGR last month:
SELECT 
    a.client_id,
    m.username,
    sumIf(a.amount, a.type = 'bet' AND a.is_rollback = 0 AND a.is_test = 0) - 
    sumIf(a.amount, a.type = 'win' AND a.is_rollback = 0 AND a.is_test = 0) AS ggr
FROM bh_transaction_main_archive AS a
LEFT JOIN m_client AS m ON a.client_id = m.id AND a.site_id = m.site_id
WHERE a.site_id = {site_id}
    AND a.created_at_dt >= toStartOfMonth(today()) - INTERVAL 1 MONTH
    AND a.created_at_dt < toStartOfMonth(today())
    AND a.is_test = 0 AND a.is_rollback = 0
GROUP BY a.client_id, m.username
ORDER BY ggr DESC
LIMIT 1000;

### Deposits this month:
SELECT 
    toDate(created_at_dt) AS date,
    sumIf(amount, type = 'deposit' AND status IN (1, 2) AND is_test = 0) AS deposits
FROM bh_payment_archive AS pa FINAL
WHERE site_id = {site_id}
    AND created_at_dt >= toStartOfMonth(today())
    AND is_test = 0
GROUP BY date
ORDER BY date
LIMIT 1000;

### New registrations last week:
SELECT COUNT(DISTINCT id) AS new_registrations
FROM m_client
WHERE site_id = {site_id}
    AND created_at >= toUnixTimestamp(toStartOfWeek(today()) - 7)
    AND created_at < toUnixTimestamp(toStartOfWeek(today()))
    AND is_test = 0
LIMIT 1000;

### Hold from deposits (cross-table) last 30 days:
SELECT 
    (sumIf(a.amount, a.type = 'bet' AND a.is_rollback = 0 AND a.is_test = 0) - 
     sumIf(a.amount, a.type = 'win' AND a.is_rollback = 0 AND a.is_test = 0)) /
    NULLIF((SELECT sumIf(amount, type = 'deposit' AND status IN (1,2) AND is_test = 0) 
            FROM bh_payment_archive FINAL 
            WHERE site_id = {site_id} AND created_at_dt >= toDate(now()) - 30), 0) AS hold_from_deposits
FROM bh_transaction_main_archive AS a
WHERE a.site_id = {site_id}
    AND a.created_at_dt >= toDate(now()) - 30
    AND a.is_test = 0 AND a.is_rollback = 0
LIMIT 1000;
`

const DevDDLSchema = `
# DATABASE SCHEMA (DDL) v1.2.2

## Bet/Win Transactions (archive)

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| after_balance | Nullable(Float64) | Player balance after transaction. | amount | YES | NO |
| amount | Float64 | Raw transaction amount (stake or win). | amount | NO | NO |
| base_amount | Nullable(Float64) | Amount converted to base currency. | amount | YES | NO |
| before_balance | Nullable(Float64) | Player balance before transaction. | amount | YES | NO |
| bet_type | String | Bet type/category (e.g., single, combo, etc.). | string | NO | NO |
| btag | Nullable(String) | Tracking tag (affiliate/campaign). |  | YES | NO |
| client_bonus_id | Nullable(UInt64) | Link to client_bonus if applicable. | id | YES | NO |
| client_id | UInt64 | Player identifier. | identifier | NO | NO |
| created_at | Date | Calendar date of transaction (partition key). | date | NO | NO |
| created_at_dt | DateTime | Full datetime of transaction. | datetime | NO | NO |
| created_at_ts | UInt64 | Timestamp in epoch microseconds. |  | NO | YES |
| currency_id | UInt32 | Currency identifier. | id | NO | NO |
| debit_id | Nullable(UInt32) | Internal accounting/debit reference. | id | YES | NO |
| game_id | Nullable(String) | Game identifier from vendor. | id | YES | NO |
| id | UInt64 | Unique transaction row ID (bet or win). | id | NO | NO |
| internal_site_game_id | Nullable(Int32) | Internal ID of the game per site. | id | YES | NO |
| is_bonus | Bool | Transaction using bonus funds. | flag | NO | NO |
| is_free_round | Bool | Marks free round / freespin. | flag | NO | NO |
| is_rollback | Bool | Marks rollback/reversal transaction. | flag | NO | NO |
| is_test | Bool | Marks test/QA transactions. | flag | NO | NO |
| meta | Nullable(String) | JSON/metadata from platform/game. | metadata | YES | YES |
| product_id | Nullable(Int32) | Product/vertical ID. | id | YES | NO |
| rates | Nullable(String) | Serialized FX rates used for conversion. | metadata | YES | YES |
| round_id | Nullable(String) | Game round identifier. | id | YES | NO |
| site_id | UInt32 | Site/brand identifier. | id | NO | NO |
| sub_vendor_id | Nullable(Int32) | Sub-vendor/studio ID. | id | YES | NO |
| table_id | UInt64 | Internal reference to source table/type. | id | NO | NO |
| type | Enum8('bet'=-1,'win'=1) | Transaction type: bet or win. | category | NO | NO |
| vendor_id | UInt32 | Game provider/vendor ID. | id | NO | NO |

## Legacy Client Table (client) [OUT OF SCOPE]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| active | UInt32 | Active flag. |  | NO | NO |
| created_at | UInt64 | Creation timestamp (epoch). | date | NO | NO |
| currency_id | UInt32 | Default currency of client. | id | NO | NO |
| email_verified | Bool | Email verification flag. | flag | NO | NO |
| id | UInt64 | Legacy client ID. | id | NO | NO |
| is_locked | Bool | Indicates locked account. | flag | NO | NO |
| is_test | Bool | Marks test clients. | flag | NO | NO |
| last_visit | UInt64 | Last visit timestamp. |  | NO | NO |
| phone_verified | Bool | Phone verification flag. | flag | NO | NO |
| site_id | UInt64 | Legacy site identifier. | id | NO | NO |
| status | UInt32 | Status code. | category | NO | NO |
| username | Nullable(String) | Player username. | username | YES | NO |
| verified | Bool | KYC verification flag. | flag | NO | NO |

## Client Accounts (client_account) [IN SCOPE (Phase 2)]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| balance | Decimal(12,2) | Current account balance. | amount | NO | NO |
| client_id | UInt32 | Owner client ID. | identifier | NO | NO |
| created_at | Int32 | Account creation timestamp. | date | NO | NO |
| id | UInt32 | Account ID per client. | id | NO | NO |
| initial_balance | Decimal(12,2) | Starting balance for rollover. | amount | NO | NO |
| is_expired | UInt8 | Marks expired account. | flag | NO | NO |
| is_main | UInt8 | Marks main wallet account. | flag | NO | NO |
| is_rollovered | UInt8 | Marks rollover completed. | flag | NO | NO |
| rollover | Decimal(12,2) | Remaining rollover requirement. |  | NO | NO |
| type | Enum8('casino'=1,'sport'=2) | Account type by vertical. | category | NO | NO |

## Client Bonuses (client_bonus) [IN SCOPE (Phase 2)]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| account_type | String | Type of account (casino/sport). | string | NO | NO |
| applied | UInt8 | Whether bonus applied. |  | NO | NO |
| balance | Decimal(12,2) | Current bonus balance. | amount | NO | NO |
| client_account_id | UInt64 | Linked client_account ID. | id | NO | NO |
| client_id | UInt64 | Player ID owning the bonus. | identifier | NO | NO |
| created_at | UInt64 | Bonus creation time. | date | NO | NO |
| expiration_date | Int32 | Expiration timestamp/date. |  | NO | NO |
| expired_at | UInt64 | Timestamp when bonus expired. | date | NO | NO |
| factor | Int16 | Bonus factor/coefficient. |  | NO | NO |
| free_round_id | UInt64 | Linked free round campaign ID. | id | NO | NO |
| given_by | UInt64 | ID of source (admin campaign, etc.). |  | NO | NO |
| id | UInt64 | Bonus instance ID per player. | id | NO | NO |
| initial_amount | Decimal(12,2) | Starting bonus amount. | amount | NO | NO |
| is_acquired | UInt8 | Whether bonus has been claimed. | flag | NO | NO |
| is_active | UInt8 | Whether bonus is active. | flag | NO | NO |
| is_congregate | UInt8 | Aggregated bonuses flag. | flag | NO | NO |
| is_expired | UInt8 | Expired status. | flag | NO | NO |
| is_exported | UInt8 | Marks if exported to external systems. | flag | NO | NO |
| is_maximun_amount_acquired | UInt8 | Max amount reached flag. | amount | NO | NO |
| is_rollover_finished | UInt8 | Rollover completion flag. | flag | NO | NO |
| is_type_rollover | UInt8 | Indicates rollover-type bonus. | flag | NO | NO |
| max_acquire_percent | UInt64 | Max bonus acquisition percentage. |  | NO | NO |
| maximun_acquire_amount | Decimal(11,2) | Max bonus amount allowed. | amount | NO | NO |
| read_status | UInt8 | Read/unread indicator for UI. |  | NO | NO |
| rollover_amount | Decimal(12,2) | Required rollover total. | amount | NO | NO |
| rollover_percent | Decimal(11,7) | Progress percent toward rollover. |  | NO | NO |
| rollovered_amount | Decimal(12,2) | Amount already rolled over. | amount | NO | NO |
| site_bonus_id | UInt64 | Site-level bonus configuration ID. | id | NO | NO |
| status | String | Bonus status (e.g., active, used). | category | NO | NO |
| updated_at | UInt64 | Last update time. | date | NO | NO |
| vendor_segment_id | UInt16 | Vendor segment for bonus. | id | NO | NO |

## Client Products (client_product) [IN SCOPE (Phase 2)]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| client_id | UInt64 | Player ID. | identifier | NO | NO |
| id | UInt64 | Row ID. | id | NO | NO |
| product_alias | String | Human-readable alias for product. | string | NO | NO |
| product_id | Int32 | Product ID (casino/sport). | id | NO | NO |
| updated_at | UInt64 | Last update timestamp. | date | NO | NO |

## Client Tags Link (client_tag_client) [IN SCOPE (Phase 2)]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| client_id | UInt32 | Player ID. | identifier | NO | NO |
| client_tag_id | UInt64 | Tag ID. | id | NO | NO |
| created_at | Nullable(UInt64) | Tag assignment created time. | date | YES | NO |
| id | UInt64 | Tag mapping row ID. | id | NO | NO |
| updated_at | Nullable(UInt64) | Last update time. | date | YES | NO |

## Currency (currency)

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| code | String | Currency code (e.g. AMD, EUR). | category | NO | NO |
| id | UInt64 | Currency ID. | id | NO | NO |
| value | Float32 | Relative value / rate or config value. |  | NO | NO |

## Exchange / Rates (exchange) [IN SCOPE (Phase 2)]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| code | String | Currency/exchange code. | category | NO | NO |
| currency_id | UInt32 | Currency ID. | id | NO | NO |
| id | Int32 | Exchange rate row ID. | id | NO | NO |
| is_enabled | Bool | Whether rate is active. | flag | NO | NO |
| is_main | Bool | Marks main/base rate for site. | flag | NO | NO |
| name | String | Display name of currency/exchange. | string | NO | NO |
| rate | Float32 | FX rate. |  | NO | NO |
| site_id | UInt64 | Site-specific exchange config. | id | NO | NO |

## Game Reference (game) [IN SCOPE (Phase 2)]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| game_id | Nullable(String) | Vendor game ID. | id | YES | NO |
| id | Nullable(String) | Internal game ID. | id | YES | NO |
| title | String | Game title. | string | NO | NO |

## Client Table (m_client)

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| active | Bool | Active flag. | flag | NO | NO |
| activity_level | Int16 | Activity level scoring. |  | NO | NO |
| client_info_id | UInt32 | Link to additional client info. | id | NO | NO |
| created_at | UInt32 | Registration timestamp. | date | NO | NO |
| currency_id | UInt16 | Default currency ID. | id | NO | NO |
| deleted_at | UInt32 | Soft-delete timestamp. | date | NO | NO |
| email_verified | Bool | Email verification flag. | flag | NO | NO |
| id | UInt32 | Canonical player ID. | identifier | NO | NO |
| ip | String | Registration or last IP. | pii_identifier | NO | NO |
| is_locked | Bool | Locked account flag. | flag | NO | NO |
| is_test | Bool | Test account flag. | flag | NO | NO |
| last_visit | UInt32 | Last visit timestamp. |  | NO | NO |
| locked | String | Locked reason/meta. | string | NO | NO |
| meta | String | JSON metadata. | metadata | NO | YES |
| phone_verified | Bool | Phone verification flag. | flag | NO | NO |
| site_id | UInt32 | Site/brand ID. | id | NO | NO |
| status | Int16 | Account status code. | category | NO | NO |
| username | String | Player username. | username | NO | NO |
| verified | Bool | KYC verified flag. | flag | NO | NO |

## Deposit/Withdraw Transactions (payment_archive_raw)

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| action_count | Nullable(UInt32) | Actions count tied to transaction. |  | YES | NO |
| after_balance | Nullable(Float64) | Balance after payment. | amount | YES | NO |
| amount | Float64 | Transaction amount in payment currency. | amount | NO | NO |
| base_amount | Float64 | Normalized/base currency amount. | amount | NO | NO |
| before_balance | Nullable(Float64) | Balance before payment. | amount | YES | NO |
| bind | Bool | Binding flag to something else. | flag | NO | NO |
| btag | Nullable(String) | Tracking tag code. |  | YES | NO |
| cashback_id | Nullable(UInt32) | Linked cashback. | id | YES | NO |
| client_account_id | UInt32 | Account used for transaction. | id | NO | NO |
| client_account_type | String | Account type (e.g. main, bonus). | string | NO | NO |
| client_bonus_id | Nullable(UInt32) | Linked client bonus. | id | YES | NO |
| client_id | UInt64 | Player ID. | identifier | NO | NO |
| created_at | Date | Creation date. | date | NO | NO |
| created_at_dt | DateTime | Payment creation datetime. | datetime | NO | NO |
| created_at_ts | UInt64 | Creation timestamp (epoch). |  | NO | YES |
| currency_code | String | Currency code. | category | NO | NO |
| currency_id | UInt32 | Currency ID. | id | NO | NO |
| external_transaction_id | Nullable(String) | PSP/reference external ID. | id | YES | NO |
| id | UInt64 | Unique payment row ID. | id | NO | NO |
| info | Nullable(String) | Additional info text. | metadata | YES | YES |
| is_correction | Bool | Correction-type payment. | flag | NO | NO |
| is_land_based | Bool | Land-based vs online. | flag | NO | NO |
| is_test | Bool | Test payment flag. | flag | NO | NO |
| meta | Nullable(String) | Extra metadata. | metadata | YES | YES |
| microtime | Float64 | Versioning field for ReplacingMergeTree. |  | NO | YES |
| rates | Nullable(String) | FX rates JSON. | metadata | YES | YES |
| ref_transaction_id | Nullable(UInt32) | Reference to other transaction. | id | YES | NO |
| settled_at | Nullable(Date) | Date when settlement occurred. | date | YES | NO |
| settled_at_dt | Nullable(DateTime) | Settlement datetime. | datetime | YES | NO |
| settled_at_ts | Nullable(UInt64) | Settlement timestamp. |  | YES | NO |
| site_bonus_action_type | Nullable(String) | Type of bonus action. |  | YES | NO |
| site_bonus_id | Nullable(UInt32) | Linked site bonus. | id | YES | NO |
| site_id | UInt32 | Site/brand ID. | id | NO | NO |
| site_payment_id | UInt32 | Payment method ID per site. | id | NO | NO |
| site_payment_type | Enum8('system'=1,'not_system'=2) | Classification of payment type. |  | NO | NO |
| status | UInt32 | Payment status code. | category | NO | NO |
| transaction_id | String | Internal transaction identifier. | id | NO | NO |
| type | Enum8('withdraw'=-1,'deposit'=1) | Payment direction. | category | NO | NO |
| updated_at | Date | Last update date. | date | NO | NO |
| updated_at_dt | DateTime | Update datetime. | datetime | NO | NO |
| updated_at_ts | UInt64 | Update timestamp. |  | NO | YES |
| withdraw_fee_amount | Nullable(Float64) | Fee amount taken. | amount | YES | NO |
| withdraw_fee_percent | Nullable(Float64) | Withdrawal fee percent. |  | YES | NO |

## Payment Archive (RabbitMQ Ingest) (payment_archive_rb) [OUT OF SCOPE]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| action_count | Nullable(UInt32) |  |  |  | NO |
| after_balance | Nullable(Float64) |  |  |  | NO |
| amount | Float64 | Transaction amount in payment currency. | amount | NO | NO |
| base_amount | Float64 | Normalized/base currency amount. | amount | NO | NO |
| before_balance | Nullable(Float64) |  |  |  | NO |
| bind | Bool |  |  |  | NO |
| btag | Nullable(String) |  |  |  | NO |
| cashback_id | Nullable(UInt32) |  |  |  | NO |
| client_account_id | UInt32 |  |  |  | NO |
| client_account_type | String |  |  |  | NO |
| client_bonus_id | Nullable(UInt32) |  |  |  | NO |
| client_id | UInt64 | Player ID. | identifier | NO | NO |
| created_at | Date | Creation date. | date | NO | NO |
| created_at_dt | DateTime | Payment creation datetime. | datetime | NO | NO |
| created_at_ts | UInt64 |  |  |  | NO |
| currency_code | String | Currency code. | category | NO | NO |
| currency_id | UInt32 | Currency ID. | id | NO | NO |
| external_transaction_id | Nullable(String) |  |  |  | NO |
| id | UInt64 | Unique payment ingest row ID. | id | NO | NO |
| info | Nullable(String) |  |  |  | NO |
| is_correction | Bool | Correction-type payment. | flag | NO | NO |
| is_land_based | Bool |  |  |  | NO |
| is_test | Bool | Test payment flag. | flag | NO | NO |
| meta | Nullable(String) |  |  |  | NO |
| microtime | Float64 |  |  |  | NO |
| rates | Nullable(String) |  |  |  | NO |
| ref_transaction_id | Nullable(UInt32) |  |  |  | NO |
| settled_at | Nullable(Date) | Date when settlement occurred. | date | YES | NO |
| settled_at_dt | Nullable(DateTime) | Settlement datetime. | datetime | YES | NO |
| settled_at_ts | Nullable(UInt64) |  |  |  | NO |
| site_bonus_action_type | Nullable(String) |  |  |  | NO |
| site_bonus_id | Nullable(UInt32) |  |  |  | NO |
| site_id | UInt32 | Site/brand ID. | id | NO | NO |
| site_payment_id | UInt32 | Payment method ID per site. | id | NO | NO |
| status | UInt32 | Payment status code. | category | NO | NO |
| transaction_id | String | Internal transaction identifier. | id | NO | NO |
| type | Enum8('withdraw'=-1,'deposit'=1) | Payment direction. | category | NO | NO |
| updated_at | Date |  |  |  | NO |
| updated_at_dt | DateTime |  |  |  | NO |
| updated_at_ts | UInt64 |  |  |  | NO |
| withdraw_fee_amount | Nullable(Float64) |  |  |  | NO |
| withdraw_fee_percent | Nullable(Float64) |  |  |  | NO |

## Products (products)

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| alias | String | Short alias (casino, sports, etc.). | string | NO | NO |
| id | Nullable(Int32) | Product ID. | id | YES | NO |
| name | String | Product full name. | string | NO | NO |

## Segment Membership (Temp) (segment_client_tmp) [IN SCOPE (Phase 2)]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| client_id | UInt32 | Player ID. | identifier | NO | NO |
| segment_id | UInt64 | Segment ID. | id | NO | NO |
| type | String | Segment type/category. | category | NO | NO |

## Site Games (site_game)

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| deleted_at | Nullable(DateTime) | Soft delete time. | date | YES | NO |
| hide | Bool | Whether game hidden from UI. | flag | NO | NO |
| id | Int32 | Site game ID. | id | NO | NO |
| img | String | Full image URL/path. | string | NO | NO |
| img_thumb | String | Thumbnail URL/path. | string | NO | NO |
| internal_game_id | UInt64 | Internal game ID. | id | NO | NO |
| is_active | Bool | Whether game is active. | flag | NO | NO |
| is_bonus_supported | Bool | Whether bonus works on this game. | flag | NO | NO |
| is_enabled | Bool | Whether game is enabled. | flag | NO | NO |
| is_free_round_supported | Bool | Whether freespins allowed. | flag | NO | NO |
| is_main | Bool | Marks main game mapping. | flag | NO | NO |
| product_id | Nullable(UInt64) | Product/vertical ID. | id | YES | NO |
| site_id | UInt64 | Site ID. | id | NO | NO |
| sub_vendor_id | UInt64 | Sub-vendor/studio ID. | id | NO | NO |
| title | String | Game title. | string | NO | NO |
| vendor_id | UInt64 | Vendor/provider ID. | id | NO | NO |

## Game Tags Link (site_game_site_tag) [IN SCOPE (Phase 2)]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| id | UInt64 | Row ID. | id | NO | NO |
| site_game_id | Int32 | Linked site_game ID. | id | NO | NO |
| site_tag_id | UInt64 | Tag ID. | id | NO | NO |

## Payment Methods (site_payment)

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| background_image | String | Background image URL/path. | string | NO | NO |
| created_at | DateTime | Creation datetime. | datetime | NO | NO |
| deposit_info | String | Info shown for deposits. | string | NO | NO |
| id | UInt32 | Payment method ID. | id | NO | NO |
| information_notice | String | Info notice text. | string | NO | NO |
| is_active | UInt8 | Active flag. | flag | NO | NO |
| is_active_deposit | UInt8 | Deposit enabled flag. | flag | NO | NO |
| is_active_payout | UInt8 | Withdrawal enabled flag. | flag | NO | NO |
| is_country_detached | UInt8 | Country-independent config flag. | flag | NO | NO |
| is_dashboard_deposit | UInt8 | Shown in dashboard deposits. | flag | NO | NO |
| is_dashboard_withdraw | UInt8 | Shown in dashboard withdrawals. | flag | NO | NO |
| is_main_config | UInt8 | Marks main config. | flag | NO | NO |
| is_online_deposit | UInt8 | Online deposit flag. | flag | NO | NO |
| is_online_payout | UInt8 | Online withdrawal flag. | flag | NO | NO |
| is_single_payout | Nullable(UInt8) | Single payout flag. | flag | YES | NO |
| is_visible | UInt8 | Visibility flag. | flag | NO | NO |
| name | String | Payment method display name. | string | NO | NO |
| order | Int32 | Display ordering. |  | NO | NO |
| payment_id | UInt32 | Global payment provider ID. | id | NO | NO |
| payout_info | String | Info shown for payouts. | string | NO | NO |
| rollover_factor | UInt32 | Rollover factor applied on deposits. |  | NO | NO |
| settings | String | JSON config. | metadata | NO | YES |
| show_notice | UInt8 | Whether to show notice. |  | NO | NO |
| site_id | UInt32 | Site/brand ID. | id | NO | NO |
| slug | String | Slug/identifier. | string | NO | NO |
| updated_at | DateTime | Update datetime. | datetime | NO | NO |
| visible_in_control | String | Control visibility settings. | string | NO | NO |

## Site Tags (site_tag) [IN SCOPE (Phase 2)]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| id | UInt64 | Tag ID. | id | NO | NO |
| name | String | Tag name. | string | NO | NO |
| site_id | UInt64 | Site/brand ID. | id | NO | NO |

## Sub Vendors (sub_vendor)

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| id | Nullable(Int32) | Sub-vendor/studio ID. | id | YES | NO |
| site_id | UInt32 | Site/brand ID. | id | NO | NO |
| title | String | Studio name. | string | NO | NO |
| vendor_segment_id | UInt32 | Vendor segment ID. | id | NO | NO |

## Sub Vendors (Test) (sub_vendor_test) [OUT OF SCOPE]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| id | Nullable(Int32) | Test sub-vendor ID. | id | YES | NO |
| site_id | UInt32 | Site ID. | id | NO | NO |
| title | String | Test studio name. | string | NO | NO |

## Testing Table (test_table) [OUT OF SCOPE]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| message | String | Test message string. | string | NO | NO |
| metric | Float32 | Test metric. |  | NO | NO |
| timestamp | DateTime | Timestamp of test record. | datetime | NO | NO |
| user_id | UInt32 | Test user ID. | id | NO | NO |

## Transaction Payment Legacy Table (transaction_payment) [OUT OF SCOPE]

| column | type | description | semantic_type | nullable | exclude_from_filters |
|--------|------|-------------|---------------|----------|---------------------|
| amount | Nullable(Decimal(12,2)) | Amount. | amount | YES | NO |
| base_amount | Nullable(Decimal(15,5)) | Base currency amount. | amount | YES | NO |
| base_amount_old | Nullable(Decimal(12,2)) | Old base amount. | amount | YES | NO |
| bind | UInt8 | Binding flag. |  | NO | NO |
| client_account_id | Nullable(UInt32) | Account ID. | id | YES | NO |
| client_id | Nullable(UInt32) | Player ID. | identifier | YES | NO |
| created_at | Nullable(UInt32) | Creation timestamp. | date | YES | NO |
| currency_id | Nullable(UInt16) | Currency ID. | id | YES | NO |
| exported | Nullable(UInt8) | Exported flag. |  | YES | NO |
| external_transaction_id | Nullable(String) | External PSP ID. | id | YES | NO |
| host | Nullable(UInt32) | Host ID. |  | YES | NO |
| id | UInt32 | Legacy payment row ID. | id | NO | NO |
| is_correction | Nullable(UInt8) | Correction flag. | flag | YES | NO |
| is_land_based | Nullable(UInt8) | Land-based flag. | flag | YES | NO |
| is_test | Nullable(UInt8) | Test payment flag. | flag | YES | NO |
| settled_at | Nullable(UInt32) | Settlement timestamp. | date | YES | NO |
| site_id | Nullable(UInt32) | Site ID. | id | YES | NO |
| site_payment_id | Nullable(UInt32) | Payment method ID. | id | YES | NO |
| status | UInt16 | Status code. | category | NO | NO |
| transaction_balance_log_id | Nullable(UInt64) | Reference to balance log. | id | YES | NO |
| transaction_id | Nullable(String) | Transaction identifier. | id | YES | NO |
| transaction_payment_info_id | Nullable(UInt32) | Link to additional info. | id | YES | NO |
| transaction_rate_log_id | Nullable(UInt64) | Reference to rate log. | id | YES | NO |
| type | Nullable(Enum8('-1'=-1,'1'=1)) | Direction (deposit/withdraw). | category | YES | NO |
| updated_at | Nullable(UInt32) | Update timestamp. | date | YES | NO |
`
