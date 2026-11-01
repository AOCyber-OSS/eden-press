# Objective 9 — Phased function coverage (planning inventory)

Denominator policy: Wave 0 pins a dated Microsoft "functions by category" snapshot
into `conformance/corpus/xlsxformula/inventory.json`. Until then, use this working
list. Counts are named worksheet functions (not operators).

## Already shipped (PR #11)

`SUM`, `AVERAGE`, `MIN`, `MAX` — plus operators `+ - * /`, unary `-`, `( )`,
`A1`, `Sheet!A1`, ranges in aggregate args.

## Phase A — daily driver (≥40 named functions)

Ship these before claiming "useful Sheets editing" beyond totals.

### Logical
`IF`, `AND`, `OR`, `NOT`, `TRUE`, `FALSE`, `IFERROR`, `IFNA`, `IFS`, `SWITCH`, `XOR`

### Math & Trig (common)
`ABS`, `SIGN`, `INT`, `TRUNC`, `ROUND`, `ROUNDUP`, `ROUNDDOWN`, `CEILING`,
`FLOOR`, `MOD`, `POWER`, `SQRT`, `EXP`, `LN`, `LOG`, `LOG10`, `PI`, `SUM`
(already), `PRODUCT`, `QUOTIENT`, `GCD`, `LCM`

### Statistical (common)
`AVERAGE` (already), `AVERAGEA`, `COUNT`, `COUNTA`, `COUNTBLANK`, `MAX`/`MIN`
(already), `MAXA`, `MINA`, `MEDIAN`, `MODE.SNGL` (or `MODE`)

### Text
`LEFT`, `RIGHT`, `MID`, `LEN`, `TRIM`, `UPPER`, `LOWER`, `PROPER`, `CONCAT`,
`CONCATENATE`, `TEXTJOIN`, `SUBSTITUTE`, `REPLACE`, `FIND`, `SEARCH`, `REPT`,
`VALUE`, `T`, `CHAR`, `CODE`

### Information
`ISBLANK`, `ISNUMBER`, `ISTEXT`, `ISLOGICAL`, `ISERR`, `ISERROR`, `ISNA`,
`ISNONTEXT`, `N`, `TYPE`, `NA`

Operators added in grammar wave (not counted as named functions): comparisons,
`&` concatenation.

## Phase B — analyst core (≥100 cumulative)

### Date & Time
`DATE`, `DATEVALUE`, `TIME`, `TIMEVALUE`, `TODAY`, `NOW`, `YEAR`, `MONTH`,
`DAY`, `HOUR`, `MINUTE`, `SECOND`, `WEEKDAY`, `WEEKNUM`, `EOMONTH`, `EDATE`,
`DAYS`, `DAYS360`, `NETWORKDAYS`, `NETWORKDAYS.INTL`, `WORKDAY`, `YEARFRAC`

### Lookup & Reference
`INDEX`, `MATCH`, `VLOOKUP`, `HLOOKUP`, `XLOOKUP`, `CHOOSE`, `COLUMN`,
`COLUMNS`, `ROW`, `ROWS`, `ADDRESS`, `HYPERLINK` (display-only / stub policy TBD)

Deferred with security note: `INDIRECT`, `OFFSET`.

### Conditional aggregates
`SUMIF`, `SUMIFS`, `COUNTIF`, `COUNTIFS`, `AVERAGEIF`, `AVERAGEIFS`,
`MAXIFS`, `MINIFS`

## Phase C — breadth (200–300 cumulative)

Pull from remaining Math/Statistical/Financial/Engineering/Text/Lookup names
present in the pinned inventory, prioritizing excelize/werkbook overlap for
test-oracle convenience. Skip compatibility aliases when a modern name exists
unless a corpus case requires the alias.

## Hard outs (this objective)

- LAMBDA family: `LAMBDA`, `LET` (maybe reconsider LET alone later), `MAP`,
  `REDUCE`, `SCAN`, `BYROW`, `BYCOL`, `MAKEARRAY`, `ISOMITTED`
- Dynamic array spills as first-class grid layout: `FILTER`, `UNIQUE`, `SORT`,
  `SORTBY`, `SEQUENCE`, `RANDARRAY`, `HSTACK`, `VSTACK`, `TOCOL`, `TOROW`, …
- Cube / OLAP, `WEBSERVICE`, `FILTERXML`, `STOCK*` linked types
- VBA / macros / XLM
- Iterative circular calculation (keep `#CYCLE!`)
- External workbook references (`[Book.xlsx]Sheet!A1`)

## Error codes the grid already or should surface

| Code | Meaning |
|------|---------|
| `#DIV/0!` | Division by zero |
| `#NAME?` | Unknown function / named range |
| `#REF!` | Bad sheet or ref |
| `#VALUE!` | Wrong type / blank where number required |
| `#NUM!` | Numeric domain error |
| `#N/A` | Lookup miss / `NA()` |
| `#CYCLE!` | Circular reference (eden extension of Excel's behavior) |
| `#ERR` | Parse failure (eden; prefer narrowing over time) |
