package openai

// SemanticDictionary contains the complete semantic dictionary for query generation
// This is the single source of truth for all table, metric, dimension, join, and alias definitions
const SemanticDictionary = `
# SEMANTIC DICTIONARY v1.2 - CANONICAL REFERENCE

This semantic dictionary is the SINGLE SOURCE OF TRUTH for all SQL generation.
You MUST only use tables, columns, metrics, dimensions, joins, and aliases defined here.
DO NOT invent or assume any entities not explicitly listed.

---

## 1. TABLES (IN SCOPE ONLY)

### 1.1 bh_transaction_main_archive (Bet/Win Transactions)
- **Engine**: MergeTree
- **Description**: Canonical fact table for bets & wins. type='bet' = stake, type='win' = payout.
- **Grain**: One row per bet/win transaction
- **Primary Key**: id
- **Business Keys**: client_id, site_id, currency_id, internal_site_game_id, vendor_id, sub_vendor_id
- **Important Columns**: type, amount, base_amount, is_bonus, is_test, is_rollback, created_at_dt
- **Enforce Site Match**: YES - ALWAYS filter by site_id
- **Default Filters**: Exclude is_test=1 AND is_rollback=1 from all KPIs
- **Time Column**: Prefer created_at_dt (DateTime)
- **Aliases**: archive, bh_transaction_main_archive

### 1.2 bh_payment_archive (Deposit/Withdraw Transactions)
- **Engine**: ReplacingMergeTree (use FINAL keyword)
- **Description**: All deposit & withdrawal transactions in their final state.
- **Grain**: One row per payment transaction
- **Primary Key**: id
- **Business Keys**: client_id, site_id, currency_id, site_payment_id
- **Important Columns**: type, amount, base_amount, status, created_at_dt, settled_at_dt, is_test
- **Enforce Site Match**: YES - ALWAYS filter by site_id
- **Status Filter**: Success statuses are status IN (1, 2) for deposits/withdrawals KPIs
- **Default Filters**: Exclude is_test=1
- **Time Column**: Prefer created_at_dt (DateTime)
- **Aliases**: payment_archive_raw, bh_payment_archive
- **FINAL Syntax**: When using FINAL with alias: FROM bh_payment_archive AS pa FINAL (put alias BEFORE FINAL)

### 1.3 m_client (Client Table)
- **Engine**: MySQL
- **Description**: Canonical player table for reporting. Use this, NOT the legacy 'client' table.
- **Grain**: One row per player
- **Primary Key**: id
- **Business Keys**: username, site_id
- **Important Columns**: currency_id, activity_level, is_test, last_visit, created_at
- **Enforce Site Match**: YES - ALWAYS filter by site_id
- **Has Deleted Flag**: YES (deleted_at)
- **Default Filters**: Exclude is_test=1
- **Time Column**: created_at (UInt32 epoch seconds)

### 1.4 currency (Currency Reference)
- **Engine**: MySQL
- **Description**: Currency reference table for display.
- **Grain**: One row per currency
- **Primary Key**: id
- **Important Columns**: code, value
- **Enforce Site Match**: NO

### 1.5 products (Products/Verticals)
- **Engine**: MySQL
- **Description**: Gaming products/verticals (casino, sports, etc.)
- **Grain**: One row per product
- **Primary Key**: id
- **Important Columns**: name, alias
- **Enforce Site Match**: NO

### 1.6 site_game (Site Games)
- **Engine**: MySQL
- **Description**: Canonical mapping of internal_game_id → game title & vendor
- **Grain**: One row per game per site
- **Primary Key**: id
- **Business Keys**: site_id, internal_game_id
- **Important Columns**: title, vendor_id, product_id, is_active
- **Enforce Site Match**: YES - ALWAYS filter by site_id
- **Has Deleted Flag**: YES (deleted_at)
- **Note**: Type conversion needed for internal_site_game_id (Int32) vs internal_game_id (UInt64)

### 1.7 sub_vendor (Sub Vendors/Studios)
- **Engine**: MySQL
- **Description**: Sub-vendors / studios
- **Grain**: One row per studio
- **Primary Key**: id
- **Important Columns**: title
- **Enforce Site Match**: YES - Join requires site_id

### 1.8 site_payment (Payment Methods)
- **Engine**: MySQL
- **Description**: Payment method catalog per site
- **Grain**: One row per payment method
- **Primary Key**: id
- **Important Columns**: name, is_active
- **Enforce Site Match**: YES - ALWAYS filter by site_id

---

## 2. OUT OF SCOPE TABLES (DO NOT USE)

- **client**: Legacy client table - USE m_client INSTEAD
- **test_table**: Testing table - DO NOT USE
- **transaction_payment**: Legacy payment table - USE bh_payment_archive INSTEAD
- **sub_vendor_test**: Test table - DO NOT USE
- **payment_archive_rb**: RabbitMQ ingest table - USE bh_payment_archive INSTEAD

---

## 3. METRICS (Canonical Formulas)

### 3.1 bets_count (Bets Count)
- **Description**: Count of real bets
- **Fact Table**: bh_transaction_main_archive
- **Formula**: countIf(type='bet' AND is_rollback=0 AND is_test=0)
- **Type**: integer
- **Synonyms**: bets, number of bets

### 3.2 bets_amount (Bets Amount / Turnover)
- **Description**: Total stake volume
- **Fact Table**: bh_transaction_main_archive
- **Formula**: sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0)
- **Type**: currency
- **Synonyms**: turnover, stakes, total stakes, bet volume, betting volume
- **Note**: For FX-adjusted views use base_amount

### 3.3 wins_amount (Wins Amount / Payouts)
- **Description**: Total payouts to players
- **Fact Table**: bh_transaction_main_archive
- **Formula**: sumIf(amount, type='win' AND is_rollback=0 AND is_test=0)
- **Type**: currency
- **Synonyms**: payouts, win amount, winnings, player winnings

### 3.4 ggr (Gross Gaming Revenue)
- **Description**: Profit before bonuses/taxes = Bets - Wins
- **Fact Table**: bh_transaction_main_archive
- **Formula**: sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0) - sumIf(amount, type='win' AND is_rollback=0 AND is_test=0)
- **Type**: currency
- **Synonyms**: gross gaming revenue, gaming profit, house win, operator win, gross win
- **CRITICAL**: GGR = Total Bet - Total Win (ALWAYS use this formula)

### 3.5 ngr (Net Gaming Revenue)
- **Description**: Net gaming revenue (currently equal to GGR until costs are modeled)
- **Fact Table**: bh_transaction_main_archive
- **Formula**: sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0) - sumIf(amount, type='win' AND is_rollback=0 AND is_test=0)
- **Type**: currency
- **Synonyms**: net gaming revenue, ngr, net revenue, net win
- **Note**: Placeholder - currently identical to GGR

### 3.6 rtp (Return To Player)
- **Description**: Win ratio = wins/stakes
- **Fact Table**: bh_transaction_main_archive
- **Formula**: sumIf(amount, type='win' AND is_rollback=0 AND is_test=0) / NULLIF(sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0), 0)
- **Type**: ratio
- **Synonyms**: return to player, rtp percentage, payout percentage, payback

### 3.7 active_players_bets (Active Players)
- **Description**: Unique players with at least one bet
- **Fact Table**: bh_transaction_main_archive
- **Formula**: uniqExactIf(client_id, type='bet' AND is_rollback=0 AND is_test=0)
- **Type**: integer
- **Synonyms**: active players, active bettors, betting players, real money players

### 3.8 deposits_amount (Deposits Amount)
- **Description**: Successful deposits
- **Fact Table**: bh_payment_archive
- **Formula**: sumIf(amount, type='deposit' AND status IN (1,2) AND is_test=0)
- **Type**: currency
- **Synonyms**: deposits, deposit volume, total deposits, cash in, top ups

### 3.9 withdrawals_amount (Withdrawals Amount)
- **Description**: Successful withdrawals
- **Fact Table**: bh_payment_archive
- **Formula**: sumIf(amount, type='withdraw' AND status IN (1,2) AND is_test=0)
- **Type**: currency
- **Synonyms**: withdrawals, withdrawal volume, total withdrawals, cashouts, cash out

### 3.10 net_deposits (Net Deposits)
- **Description**: Deposits minus withdrawals
- **Fact Table**: bh_payment_archive
- **Formula**: (sum deposits) - (sum withdrawals)
- **Type**: currency
- **Synonyms**: net deposits, net cash in, cash flow

### 3.11 ftd_count (First-Time Depositors)
- **Description**: First-ever successful deposit per player
- **Fact Table**: bh_payment_archive
- **Formula**: uniqExactIf(client_id, is_first_deposit=1)
- **Type**: integer
- **Synonyms**: ftd, new depositors, first time depositors
- **Note**: Requires subquery or materialized table with is_first_deposit flag

### 3.12 ftd_amount (First-Time Deposit Amount)
- **Description**: Total amount of first successful deposits
- **Fact Table**: bh_payment_archive
- **Formula**: sumIf(base_amount, type='deposit' AND status IN (1,2) AND is_test=0 AND is_first_deposit=1)
- **Type**: currency
- **Synonyms**: ftd amount, first deposit amount, ftd value

### 3.13 avg_bet (Average Bet Amount)
- **Description**: Average stake size
- **Fact Table**: bh_transaction_main_archive
- **Formula**: sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0) / NULLIF(countIf(type='bet' AND is_rollback=0 AND is_test=0), 0)
- **Type**: currency
- **Synonyms**: average bet, average stake, avg stake

### 3.14 ggr_margin (GGR Margin / Hold)
- **Description**: House margin = GGR / Bets
- **Fact Table**: bh_transaction_main_archive
- **Formula**: (sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0) - sumIf(amount, type='win' AND is_rollback=0 AND is_test=0)) / NULLIF(sumIf(amount, type='bet' AND is_rollback=0 AND is_test=0), 0)
- **Type**: ratio
- **Synonyms**: hold, hold percentage, house edge, house margin, margin

### 3.15 unique_depositors (Unique Depositors)
- **Description**: Distinct players with successful deposits
- **Fact Table**: bh_payment_archive
- **Formula**: uniqExactIf(client_id, type='deposit' AND status IN (1,2) AND is_test=0)
- **Type**: integer
- **Synonyms**: depositors, depositing players

### 3.16 bonus_bets_amount (Bonus Bets Amount)
- **Description**: Bets made with bonus funds
- **Fact Table**: bh_transaction_main_archive
- **Formula**: sumIf(amount, type='bet' AND is_bonus=1 AND is_test=0)
- **Type**: currency
- **Synonyms**: bonus turnover, bonus stakes

### 3.17 bonus_ggr (Bonus GGR)
- **Description**: GGR from bonus play
- **Fact Table**: bh_transaction_main_archive
- **Formula**: sumIf(amount, type='bet' AND is_bonus=1 AND is_test=0) - sumIf(amount, type='win' AND is_bonus=1 AND is_test=0)
- **Type**: currency
- **Synonyms**: bonus profit

### 3.18 registrations (New Registrations)
- **Description**: Count of newly registered players
- **Fact Table**: m_client
- **Formula**: countIf(is_test=0)
- **Type**: integer
- **Synonyms**: new clients, signups, new players, new registrations, registered clients, registered players
- **Time Filter**: Use created_at column (UInt32 epoch) - convert with toUnixTimestamp()
- **Example**: For last week: WHERE created_at >= toUnixTimestamp(toStartOfWeek(today()) - 7) AND created_at < toUnixTimestamp(toStartOfWeek(today()))

---

## 4. DIMENSIONS

### 4.1 date (Date)
- **Type**: temporal
- **Source**: bh_transaction_main_archive.created_at_dt, bh_payment_bh_transaction_main_archive.created_at_dt
- **Fallback**: If created_at_dt null, use created_at (Date)

### 4.2 site (Site/Brand)
- **Type**: entity
- **Source**: bh_transaction_main_archive.site_id, bh_payment_bh_transaction_main_archive.site_id, m_client.site_id
- **Synonyms**: site, brand, operator

### 4.3 currency (Currency)
- **Type**: categorical
- **Source**: bh_transaction_main_archive.currency_id, bh_payment_bh_transaction_main_archive.currency_id
- **Lookup**: currency.id → currency.code
- **Synonyms**: currency, ccy

### 4.4 product (Product/Vertical)
- **Type**: entity
- **Source**: bh_transaction_main_archive.product_id, site_game.product_id
- **Lookup**: products.id → products.alias
- **Synonyms**: product, vertical, category

### 4.5 game (Game)
- **Type**: entity
- **Source**: bh_transaction_main_archive.internal_site_game_id → site_game.internal_game_id
- **Lookup**: site_game.internal_game_id → site_game.title
- **Synonyms**: game, title, slot, casino game
- **Note**: Cast types if needed (Int32 ↔ UInt64)

### 4.6 vendor (Vendor/Provider)
- **Type**: entity
- **Source**: bh_transaction_main_archive.vendor_id, site_game.vendor_id
- **Synonyms**: provider, vendor, game provider

### 4.7 sub_vendor (Sub Vendor/Studio)
- **Type**: categorical
- **Source**: bh_transaction_main_archive.sub_vendor_id
- **Lookup**: sub_vendor.id → sub_vendor.title
- **Synonyms**: studio, subvendor
- **Note**: Must join using site_id

### 4.8 client (Player)
- **Type**: entity
- **Source**: bh_transaction_main_archive.client_id, bh_payment_bh_transaction_main_archive.client_id, m_client.id
- **Lookup**: m_client.id → m_client.username
- **Synonyms**: player, user, client
- **PII**: username may require masking

### 4.9 payment_method (Payment Method)
- **Type**: categorical
- **Source**: bh_payment_bh_transaction_main_archive.site_payment_id
- **Lookup**: site_payment.id → site_payment.name
- **Synonyms**: psp, payment system
- **Note**: Must join using site_id

---

## 5. CANONICAL JOINS

### 5.1 bh_transaction_main_archive → m_client (Player Enrichment)
- **Join Type**: LEFT
- **Condition**: bh_transaction_main_archive.client_id = m_client.id AND bh_transaction_main_archive.site_id = m_client.site_id
- **Cardinality**: many_to_one
- **Site Match**: REQUIRED

### 5.2 bh_transaction_main_archive → currency
- **Join Type**: LEFT
- **Condition**: bh_transaction_main_archive.currency_id = currency.id
- **Cardinality**: many_to_one
- **Site Match**: Not required

### 5.3 bh_transaction_main_archive → site_game (Game Enrichment)
- **Join Type**: LEFT
- **Condition**: bh_transaction_main_archive.internal_site_game_id = site_game.internal_game_id AND bh_transaction_main_archive.site_id = site_game.site_id
- **Cardinality**: many_to_one
- **Site Match**: REQUIRED
- **Note**: Type mismatch possible (Int32 ↔ UInt64)

### 5.4 bh_transaction_main_archive → sub_vendor (Studio Enrichment)
- **Join Type**: LEFT
- **Condition**: bh_transaction_main_archive.sub_vendor_id = sub_vendor.id AND bh_transaction_main_archive.site_id = sub_vendor.site_id
- **Cardinality**: many_to_one
- **Site Match**: REQUIRED

### 5.5 bh_payment_archive → m_client
- **Join Type**: LEFT
- **Condition**: bh_payment_bh_transaction_main_archive.client_id = m_client.id AND bh_payment_bh_transaction_main_archive.site_id = m_client.site_id
- **Cardinality**: many_to_one
- **Site Match**: REQUIRED

### 5.6 bh_payment_archive → currency
- **Join Type**: LEFT
- **Condition**: bh_payment_bh_transaction_main_archive.currency_id = currency.id
- **Cardinality**: many_to_one
- **Site Match**: Not required

### 5.7 bh_payment_archive → site_payment (Payment Method)
- **Join Type**: LEFT
- **Condition**: bh_payment_bh_transaction_main_archive.site_payment_id = site_payment.id AND bh_payment_bh_transaction_main_archive.site_id = site_payment.site_id
- **Cardinality**: many_to_one
- **Site Match**: REQUIRED

---

## 6. DATE PRESETS

| Preset ID | Description | Notes |
|-----------|-------------|-------|
| today | Today only | Use toDate(now()) or today() |
| yesterday | Previous day | Use today() - 1 |
| last_7_days | Last 7 days (rolling) | Use today() - 7 to today() |
| last_30_days | Last 30 days (rolling) | Use today() - 30 to today() |
| this_week | Current calendar week | Use toStartOfWeek(today()) |
| last_week | Previous calendar week | Week before current week |
| this_month | Current calendar month | Use toStartOfMonth(today()) |
| last_month | Previous calendar month | Month before current month |
| mtd | Month to date | From start of month to today |
| ytd | Year to date | From start of year to today |

---

## 7. SEMANTIC ALIASES (Natural Language → Canonical Mapping)

### 7.1 Direct Mappings (No Clarification Needed)

#### Metrics
- "turnover", "stakes", "total stakes" → metric.bets_amount
- "cash in" → metric.deposits_amount
- "cash out" → metric.withdrawals_amount
- "ggr", "gross gaming revenue", "house win", "gaming profit" → metric.ggr
- "ngr", "net gaming revenue", "net win" → metric.ngr
- "active players", "active bettors" → metric.active_players_bets
- "deposits", "total deposits" → metric.deposits_amount
- "withdrawals", "cashouts" → metric.withdrawals_amount
- "ftd", "new depositors", "first time depositors" → metric.ftd_count
- "rtp", "return to player", "payout percentage" → metric.rtp
- "hold", "house edge", "margin" → metric.ggr_margin
- "average bet", "avg bet" → metric.avg_bet

#### Dimensions
- "country", "geo", "region" → dimension.country
- "vendor", "provider", "studio" → dimension.vendor
- "game", "title", "slot" → dimension.game
- "product", "vertical" → dimension.product
- "site", "brand", "operator" → dimension.site
- "currency", "ccy" → dimension.currency

#### Date Presets
- "today", "for today" → date_preset.today
- "yesterday", "previous day" → date_preset.yesterday
- "last 7 days", "past 7 days" → date_preset.last_7_days
- "last week", "past week" → date_preset.last_week
- "this week", "current week" → date_preset.this_week
- "last 30 days", "past 30 days" → date_preset.last_30_days
- "this month", "current month" → date_preset.this_month
- "last month" → date_preset.last_month
- "mtd", "month to date" → date_preset.mtd
- "ytd", "year to date" → date_preset.ytd

### 7.2 Ambiguous Terms (REQUIRE CLARIFICATION)

When user says these terms, ASK FOR CLARIFICATION:

- **"revenue"** → Could mean GGR (bets−wins) OR Net Deposits (cash in−cash out)
  - Ask: "Do you mean GGR (bets−wins) or Net Deposits (cash in−cash out)?"

- **"profit"** → Could mean GGR OR Net Deposits
  - Ask: "Do you mean gaming profit (GGR) or cash flow profit (Net Deposits)?"

- **"net profit"** → Could mean GGR OR NGR
  - Ask: "Do you mean GGR (gross gaming revenue) or NGR (net gaming revenue after costs)?"

- **"margin"** → Could mean GGR Margin (ratio) OR absolute GGR
  - Ask: "Do you mean GGR margin (GGR / stakes) or absolute GGR?"

- **"performance"** → Could mean GGR, NGR, or Active Players
  - Ask: "When you say performance, do you mean revenue (GGR/NGR), activity (active players), or another KPI?"

- **"activity"** → Could mean Active Players or Bets Count
  - Ask: "Do you mean number of active players, number of bets, or another activity metric?"

- **"volume"** → Could mean Bets Amount or Deposits Amount
  - Ask: "Do you mean bet volume (stakes) or deposits volume?"

### 7.3 Unsupported Terms

These terms are NOT defined in the dictionary and cannot generate SQL:

- "losing players" → unsupported
- "big players" → unsupported

Response: "I can only generate reports. Please ask me a data reporting question."

---

## 8. CRITICAL RULES

### 8.1 Default Filters (ALWAYS APPLY)
1. **bh_transaction_main_archive table**: is_test = 0 AND is_rollback = 0
2. **bh_payment_bh_transaction_main_archive table**: is_test = 0 AND status IN (1, 2) for success
3. **m_client table**: is_test = 0

### 8.2 Site Isolation (MANDATORY)
- ALWAYS include: WHERE site_id = {site_id}
- Apply to ALL tables with site_id column
- For JOINs, ensure both tables filter by site_id

### 8.3 Type-Safe Date Filtering
- **DateTime columns** (created_at_dt): Use toDateTime() functions
- **Date columns** (created_at): Use toDate() functions
- **UInt32 epoch columns**: Use toUnixTimestamp() for comparison

### 8.4 ReplacingMergeTree Tables
- For bh_payment_archive: Use FINAL keyword for consistency
- Be aware of potential duplicates in non-final queries
- **CRITICAL SYNTAX**: When using FINAL with an alias, put alias BEFORE FINAL:
  - CORRECT: FROM bh_payment_archive AS pa FINAL
  - WRONG: FROM bh_payment_archive FINAL pa
  - WRONG: FROM bh_payment_archive FINAL AS pa

### 8.5 Player-Level Queries
When returning player-level rows (not aggregated):
1. ALWAYS include client_id
2. ALWAYS include username (join to m_client if needed)

### 8.6 Column Exclusions
ALWAYS ignore these columns:
- _peerdb_synced_at
- _peerdb_is_deleted
- _peerdb_version

---

## 9. EXAMPLE QUERY PATTERNS

### GGR for last 7 days by game:
SELECT 
    sg.title as game_name,
    sumIf(a.amount, a.type = 'bet' AND a.is_rollback = 0 AND a.is_test = 0) -
    sumIf(a.amount, a.type = 'win' AND a.is_rollback = 0 AND a.is_test = 0) as ggr
FROM bh_transaction_main_archive a
LEFT JOIN site_game sg ON a.internal_site_game_id = sg.internal_game_id AND a.site_id = sg.site_id
WHERE a.site_id = {site_id}
    AND a.created_at_dt >= today() - 7
    AND a.is_test = 0
    AND a.is_rollback = 0
GROUP BY sg.title
ORDER BY ggr DESC
LIMIT 100;

### Deposits for this month:
SELECT 
    sumIf(amount, type = 'deposit' AND status IN (1, 2) AND is_test = 0) as total_deposits
FROM bh_payment_archive FINAL
WHERE site_id = {site_id}
    AND created_at_dt >= toStartOfMonth(today())
    AND is_test = 0;

### Active players by product:
SELECT 
    p.alias as product,
    uniqExactIf(a.client_id, a.type = 'bet' AND a.is_rollback = 0 AND a.is_test = 0) as active_players
FROM bh_transaction_main_archive a
LEFT JOIN products p ON a.product_id = p.id
WHERE a.site_id = {site_id}
    AND a.created_at_dt >= today() - 30
    AND a.is_test = 0
    AND a.is_rollback = 0
GROUP BY p.alias
ORDER BY active_players DESC
LIMIT 100;
`
