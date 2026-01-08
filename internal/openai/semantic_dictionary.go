package openai

// SystemPrompt contains the core instructions for the AI
const SystemPrompt = `# AI QUERY ASSISTANT - SYSTEM INSTRUCTIONS v1.2.1

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

### ReplacingMergeTree Tables
For bh_payment_archive (ReplacingMergeTree), use FINAL keyword:
- CORRECT: FROM bh_payment_archive AS pa FINAL
- WRONG: FROM bh_payment_archive FINAL pa

### GGR Formula (CRITICAL)
GGR = Bets - Wins (ALWAYS). Never calculate differently.
Formula: sumIf(amount, type='bet'...) - sumIf(amount, type='win'...)

### Column Exclusions
NEVER select: _peerdb_synced_at, _peerdb_is_deleted, _peerdb_version

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
`

// SemanticDictionary contains all the semantic mappings
const SemanticDictionary = `# SEMANTIC DICTIONARY v1.2.1

## 1. TABLES

### 1.1 IN-SCOPE TABLES

| table_id | physical_name | engine | description | site_filter | default_filters |
|----------|---------------|--------|-------------|-------------|-----------------|
| archive | bh_transaction_main_archive | MergeTree | Bet/Win transactions. type='bet'=stake, type='win'=payout | YES | is_test=0, is_rollback=0 |
| payment_archive_raw | bh_payment_archive | ReplacingMergeTree | Deposit/Withdraw transactions. Requires FINAL | YES | is_test=0, status IN (1,2) |
| m_client | m_client | MySQL | Canonical player table | YES | is_test=0 |
| currency | currency | MySQL | Currency reference | NO | - |
| products | products | MySQL | Gaming verticals (casino, sports) | NO | - |
| site_game | site_game | MySQL | Game metadata per site | YES | - |
| sub_vendor | sub_vendor | MySQL | Studio/sub-vendor info | YES | - |
| site_payment | site_payment | MySQL | Payment method catalog | YES | - |

### 1.2 OUT-OF-SCOPE TABLES (DO NOT USE)

client, test_table, transaction_payment, sub_vendor_test, payment_archive_rb

## 2. METRICS

### 2.1 Gaming Metrics (fact_table: bh_transaction_main_archive)

| metric_id | name | formula_clickhouse |
|-----------|------|--------------------|
| bets_count | Bets Count | countIf(type='bet' AND is_rollback=0 AND is_test=0) |
| bets_amount | Bets Amount | sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0) |
| wins_amount | Wins Amount | sumIf(amount, type='win' AND is_rollback=0 AND is_test=0) |
| ggr | Gross Gaming Revenue | sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0) - sumIf(amount, type='win' AND is_rollback=0 AND is_test=0) |
| ngr | Net Gaming Revenue | sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0) - sumIf(amount, type='win' AND is_rollback=0 AND is_test=0) |
| rtp | Return To Player | sumIf(amount, type='win' AND is_rollback=0 AND is_test=0) / NULLIF(sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0), 0) |
| active_players_bets | Active Players | uniqExactIf(client_id, type='bet' AND is_rollback=0 AND is_test=0) |
| avg_bet | Average Bet | sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0) / NULLIF(countIf(type='bet' AND is_rollback=0 AND is_test=0), 0) |
| ggr_margin | GGR Margin | (sumIf(amount, type='bet'...) - sumIf(amount, type='win'...)) / NULLIF(sumIf(amount, type='bet'...), 0) |
| bonus_bets_amount | Bonus Bets | sumIf(amount, type='bet' AND is_bonus=1 AND is_test=0) |
| bonus_ggr | Bonus GGR | sumIf(amount, type='bet' AND is_bonus=1...) - sumIf(amount, type='win' AND is_bonus=1...) |
| bonus_turnover | Bonus Turnover | sumIf(amount, type='bet' AND is_bonus=1 AND is_rollback=0 AND is_test=0) |

### 2.2 Payment Metrics (fact_table: bh_payment_archive)

| metric_id | name | formula_clickhouse |
|-----------|------|--------------------|
| deposits_amount | Deposits Amount | sumIf(amount, type='deposit' AND status IN (1,2) AND is_test=0) |
| withdrawals_amount | Withdrawals Amount | sumIf(amount, type='withdraw' AND status IN (1,2) AND is_test=0) |
| net_deposits | Net Deposits | deposits_amount - withdrawals_amount |
| ftd_count | First-Time Depositors | uniqExactIf(client_id, is_first_deposit=1) |
| ftd_amount | FTD Amount | sumIf(base_amount, type='deposit' AND status IN (1,2) AND is_test=0 AND is_first_deposit=1) |
| unique_depositors | Unique Depositors | uniqExactIf(client_id, type='deposit' AND status IN (1,2) AND is_test=0) |

### 2.3 Player Metrics (fact_table: m_client)

| metric_id | name | formula_clickhouse | notes |
|-----------|------|--------------------|-------|
| registered_players | Registered Players | COUNT(DISTINCT id) | created_at is UInt32 epoch |

## 3. DIMENSIONS

| dimension_id | source_table | source_column | lookup_table | display_column |
|--------------|--------------|---------------|--------------|----------------|
| date | archive/payment_archive_raw | created_at_dt | - | - |
| site | all tables | site_id | - | - |
| currency | archive/payment_archive_raw | currency_id | currency | code |
| product | archive | product_id | products | alias |
| game | archive | internal_site_game_id | site_game | title |
| vendor | archive | vendor_id | - | - |
| sub_vendor | archive | sub_vendor_id | sub_vendor | title |
| client | archive/payment_archive_raw | client_id | m_client | username |
| payment_method | payment_archive_raw | site_payment_id | site_payment | name |
| country | m_client | meta | - | country_name |

## 4. JOINS

### 4.1 bh_transaction_main_archive joins

| to_table | condition |
|----------|-----------|
| m_client | a.client_id = m_client.id AND a.site_id = m_client.site_id |
| currency | a.currency_id = currency.id |
| site_game | a.internal_site_game_id = site_game.internal_game_id AND a.site_id = site_game.site_id |
| sub_vendor | a.sub_vendor_id = sub_vendor.id AND a.site_id = sub_vendor.site_id |
| products | a.product_id = products.id |

### 4.2 bh_payment_archive joins

| to_table | condition |
|----------|-----------|
| m_client | p.client_id = m_client.id AND p.site_id = m_client.site_id |
| currency | p.currency_id = currency.id |
| site_payment | p.site_payment_id = site_payment.id AND p.site_id = site_payment.site_id |

## 5. SEMANTIC ALIASES

### 5.1 Direct Mappings (resolution_strategy = default)

**Metric Aliases:**
turnover, stakes, total stakes, bet volume -> bets_amount
bets count, number of bets, bet count -> bets_count
wins amount, total wins, winnings, payouts -> wins_amount
ggr, gross gaming revenue, gaming revenue, house win, operator win -> ggr
ngr, net gaming revenue, net win -> ngr
active players, bettors, players, betting players -> active_players_bets
deposits, deposit volume, total deposits, cash in -> deposits_amount
withdrawals, cash out, cashouts -> withdrawals_amount
ftd, new depositors, first time depositors -> ftd_count
rtp, return to player, payout ratio -> rtp
hold, ggr margin, house margin -> ggr_margin
average bet, avg bet, avg stake -> avg_bet
registrations, signups, new players, new clients, registered players -> registered_players
unique depositors, depositing players -> unique_depositors

**Dimension Aliases:**
country, geo, region, jurisdiction, market -> dimension.country
game, title, slot, casino game -> dimension.game
vendor, provider, studio, game provider -> dimension.vendor
product, vertical, category -> dimension.product
currency, ccy -> dimension.currency
payment method, psp -> dimension.payment_method

**Date Preset Aliases:**
today, for today -> today
yesterday, previous day -> yesterday
last 7 days, past 7 days -> last_7_days
last 30 days, past 30 days -> last_30_days
this week, current week -> this_week
last week, previous week -> last_week
this month, current month -> this_month
last month, previous month -> last_month
mtd, month to date -> mtd
ytd, year to date -> ytd

### 5.2 Ambiguous Terms (resolution_strategy = clarify) - MUST ASK USER

| phrase | candidate_ids | clarification |
|--------|---------------|---------------|
| revenue | ggr, net_deposits | GGR or Net Deposits? |
| profit | ggr, net_deposits | Gaming profit (GGR) or cash flow (Net Deposits)? |
| net profit, profitability | ggr, ngr | GGR or NGR? |
| margin | ggr, ggr_margin | Absolute GGR or GGR margin ratio? |
| performance | ggr, ngr, active_players_bets | Revenue or activity? |
| volume | bets_amount, deposits_amount | Bet volume or deposits? |

### 5.3 Unsupported Terms (resolution_strategy = off_topic) - REJECT

losing players, big players, whales, VIP players (until segment dimension is ready)

## 6. DATE PRESETS

| preset_id | description |
|-----------|-------------|
| today | toDate(now()) |
| yesterday | toDate(now()) - 1 AND < toDate(now()) |
| last_7_days | toDate(now()) - 7 |
| last_30_days | toDate(now()) - 30 |
| this_week | toStartOfWeek(today()) |
| last_week | toStartOfWeek(today()) - 7 AND < toStartOfWeek(today()) |
| this_month | toStartOfMonth(today()) |
| last_month | toStartOfMonth(today()) - INTERVAL 1 MONTH AND < toStartOfMonth(today()) |
| mtd | toStartOfMonth(today()) |
| ytd | toStartOfYear(today()) |

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
`

// DDLSchema contains the database schema (column definitions)
const DDLSchema = `# DATABASE SCHEMA (DDL)

## bh_transaction_main_archive (archive)

| column | type | description |
|--------|------|-------------|
| id | UInt64 | Primary key |
| client_id | UInt64 | Player identifier |
| site_id | UInt32 | Site/brand identifier |
| currency_id | UInt32 | Currency ID |
| product_id | Nullable(Int32) | Product/vertical ID |
| vendor_id | UInt32 | Game provider ID |
| sub_vendor_id | Nullable(Int32) | Studio ID |
| internal_site_game_id | Nullable(Int32) | Game ID per site |
| type | Enum8('bet'=-1,'win'=1) | Transaction type |
| amount | Float64 | Transaction amount |
| base_amount | Nullable(Float64) | Amount in base currency |
| is_bonus | Bool | Bonus funds flag |
| is_test | Bool | Test transaction flag |
| is_rollback | Bool | Rollback flag |
| created_at | Date | Date (partition key) |
| created_at_dt | DateTime | Full datetime |
| round_id | Nullable(String) | Game round ID |
| game_id | Nullable(String) | Vendor game ID |
| bet_type | String | Bet category |

## bh_payment_archive (payment_archive_raw)

| column | type | description |
|--------|------|-------------|
| id | UInt64 | Primary key |
| client_id | UInt64 | Player identifier |
| site_id | UInt32 | Site identifier |
| currency_id | UInt32 | Currency ID |
| site_payment_id | Nullable(UInt32) | Payment method ID |
| type | String | deposit or withdraw |
| amount | Float64 | Transaction amount |
| base_amount | Nullable(Float64) | Base currency amount |
| status | UInt32 | Status code (1,2 = success) |
| is_test | Bool | Test flag |
| is_first_deposit | Nullable(Bool) | First deposit flag |
| created_at_dt | DateTime | Created datetime |
| settled_at_dt | Nullable(DateTime) | Settlement datetime |

## m_client

| column | type | description |
|--------|------|-------------|
| id | UInt64 | Primary key (joins to client_id) |
| username | Nullable(String) | Player username |
| site_id | UInt32 | Site identifier |
| currency_id | UInt32 | Default currency |
| is_test | Bool | Test client flag |
| created_at | UInt32 | Registration timestamp (epoch seconds) |
| last_visit | UInt64 | Last visit timestamp |
| activity_level | Nullable(String) | Activity classification |
| meta | Nullable(String) | JSON metadata |

## currency

| column | type | description |
|--------|------|-------------|
| id | UInt32 | Primary key |
| code | String | Currency code (USD, EUR) |
| value | Float64 | Exchange rate |

## products

| column | type | description |
|--------|------|-------------|
| id | Int32 | Primary key |
| name | String | Product name |
| alias | String | Product alias (casino, sports) |

## site_game

| column | type | description |
|--------|------|-------------|
| id | UInt64 | Primary key |
| site_id | UInt32 | Site identifier |
| internal_game_id | UInt64 | Internal game ID |
| title | String | Game title |
| vendor_id | UInt32 | Vendor ID |
| product_id | Int32 | Product ID |
| is_active | Bool | Active flag |

## sub_vendor

| column | type | description |
|--------|------|-------------|
| id | Int32 | Primary key |
| site_id | UInt32 | Site identifier |
| title | String | Studio name |

## site_payment

| column | type | description |
|--------|------|-------------|
| id | UInt32 | Primary key |
| site_id | UInt32 | Site identifier |
| name | String | Payment method name |
| is_active | Bool | Active flag |
`
