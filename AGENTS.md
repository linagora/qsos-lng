# QSOS::LNG

## Project Overview

QSOS::LNG analyzes open-source projects using a **TOML-based configuration system**. It collects metrics from GitHub, Lizard, and OpenSSF Scorecard, then computes normalized 1-5 scores using declarative formulas across Community, Technical Quality, and Security categories.

**Modes:**
- `go run . analyze <owner/repo>` - One-shot analysis showing raw metrics
- `go run . work` - Background worker processing draft projects from PostgreSQL (shared with Django Argus du Libre)

**Environment Variables:**
- Required: `GITHUB_TOKEN`, `AI_API_KEY`
- Work mode: `DATABASE_URL`
- Optional: `AI_BASE_URL`, `AI_MODEL` (defaults to `gpt-oss-120b`)

## Architecture

### New TOML-Based System

**Core Packages:**
- `pkg/config/` - TOML configuration parser and validator
- `pkg/database/` - Database layer (validation, lookup cache, storage)
- `pkg/formula/` - Formula evaluator with conditional logic
- `pkg/engine/` - Sequential execution pipeline orchestrator
- `pkg/sources/` - Source adapters (GitHub, Lizard, Scorecard, Documentation, Metadata)

**Legacy Packages** (preserved for reference):
- `community/` - Original GitHub/documentation fetching code
- `tech/` - Original Lizard analysis code
- `security/` - Original Scorecard code
- `metadata/` - AI summaries, tags, icons (still used by new adapters)
- `common/` - Shared scoring utilities

### Configuration (`configs/qsos-default.toml`)

**Sections:**
1. **`[[metrics]]`** - Defines what data to fetch and metric slugs for storage
2. **`[[scores]]`** - Defines scoring formulas using metric values
3. **`[[metadata]]`** - AI enrichment operations (summaries, tags, icons)
4. **`[output]`** - Output configuration and database settings

**Example:**
```toml
# Collect stars metric
[[metrics]]
name = "github_repo_info"
fields = [
    { api_field = "stargazers_count", metric_slug = "stars" }
]

# Score popularity using formula
[[scores]]
slug = "popularity"
formula = "compute_score(metric.stars, [5000, 20000, 40000, 80000], 'bigger_is_better')"
```

### Data Flow

```
1. Load TOML config → Validate → Build metric lookup cache
2. For each draft project:
   a) Fetch metrics from sources → Store in categories_metricvalue
   b) Evaluate formulas → Store scores in categories_analysisresult
   c) Execute metadata operations → Store summaries/tags
   d) Update software state to 'in_review'
```

### Formula Language

**Built-in Functions:**
- `compute_score(value, [t1, t2, t3, t4], direction)` - Maps value to 1-5 score
- `weighted_avg([v1, v2, ...], total_weight)` - Weighted average
- `if(condition, true_value, false_value)` - Conditional logic

**Operators:**
- Arithmetic: `+`, `-`, `*`, `/`
- Comparison: `>`, `<`, `>=`, `<=`, `==`, `!=`
- Parentheses for grouping

**Metric Access:**
- `metric.slug_name` - Reads latest metric value from database

**Example with Conditionals:**
```toml
[[scores]]
slug = "scorecard"
formula = """
  1 + weighted_avg([
    metric.scorecard_vulnerabilities * 3,
    if(metric.is_mirror == 1, 0, metric.scorecard_code_review * 3),
    metric.scorecard_security_policy * 2
  ], if(metric.is_mirror == 1, 18, 21)) / 2
"""
```

### Source Adapters

**GitHub** (`pkg/sources/github.go`):
- Collects: stars, is_mirror flag
- Computes: days_since_first_commit (maturity), days_since_last_commit (activity)
- Contributors: Active in last 6 months, >3 commits, no bots

**Lizard** (`pkg/sources/lizard.go`):
- Runs Docker container with Lizard analysis
- Metrics: lines_of_code, test_lines_of_code, test_ratio, complexity (%), high_complexity_functions, functions
- Separates production and test code

**Scorecard** (`pkg/sources/scorecard.go`):
- Runs OpenSSF Scorecard via Docker
- Flattens each check into separate metric (e.g., `scorecard_vulnerabilities`)
- Skips Code-Review check for mirror repositories

**Documentation** (`pkg/sources/documentation.go`):
- README: quality score (0-100), key sections count
- Docs directory: presence and file count
- Accessibility: CONTRIBUTING.md, issue templates
- Multi-language README support

**Metadata** (`pkg/sources/metadata.go`):
- Bilingual AI summaries (French/English)
- AI-generated tags (3-5, reuses existing)
- Icon URL resolution (simple-icons → devicons fallback)

### Execution Pipeline

**Sequential Steps:**
1. **Collect Metrics** - Fetch from all sources in parallel
2. **Store Metrics** - Insert into `categories_metricvalue` with timestamps
3. **Evaluate Scores** - Execute formulas, read metrics from database
4. **Store Scores** - Insert into `categories_analysisresult`
5. **Metadata Operations** - Execute in transaction (summaries, tags, icons)
6. **Update State** - Set software state to configured value

## Database Schema

### Time-Series Metrics (New)

**`categories_metric`** - Metric definitions (created in Django admin):
- `id`, `slug`, `weight`, `collection_enabled`, `field_id`
- Metrics must be pre-defined before QSOS-LNG can collect them

**`categories_metricvalue`** - Time-series metric storage:
- `id`, `metric_id`, `software_id`, `value`, `collected_at`, `source`
- All raw metrics stored here with timestamps
- Source tracks origin: `"qsos-lng:github"`, `"qsos-lng:lizard"`, etc.

### Existing Tables

**`categories_software`** - Projects:
- `state`, `repository_url`, `logo_url`, `website_url`

**`categories_field`** - Field definitions:
- `id`, `slug` - Maps score slugs to database IDs

**`categories_analysisresult`** - Computed scores:
- `software_id`, `field_id`, `score`, `is_published`, `is_manual`, `created_at`

**`categories_block`** - Content blocks:
- `software_id`, `kind`, `locale` (fr/en), `content`

**`categories_tag`** - Tags:
- `name`, `slug` (unique)

**`categories_software_tags`** - M2M:
- `software_id`, `tag_id` (unique pair)

### Required Metric Slugs

**Community Category:**
- `stars`, `is_mirror`, `maturity_days`, `activity_days`, `active_contributors`

**Documentation:**
- `readme_quality`, `readme_sections`, `docs_directory`, `accessibility`, `multilang_readmes`

**Technical Quality:**
- `lines_of_code`, `test_lines_of_code`, `test_ratio`, `complexity`, `high_complexity_functions`, `functions`

**Security (Scorecard):**
- `scorecard_vulnerabilities`, `scorecard_security_policy`, `scorecard_binary_artifacts`
- `scorecard_branch_protection`, `scorecard_code_review`, `scorecard_pinned_dependencies`
- `scorecard_packaging`, `scorecard_signed_releases`

### Field Slugs (Score Categories)

**Community:** `maturity`, `activity`, `popularity`, `contributors`, `documentation`
**Tech:** `size`, `complexity`, `tests`
**Security:** `scorecard`

## How to Customize

### 1. Change Thresholds

Edit `configs/qsos-default.toml`:

```toml
[[scores]]
slug = "popularity"
# Change thresholds from [5000, 20000, 40000, 80000] to:
formula = "compute_score(metric.stars, [10000, 30000, 60000, 100000], 'bigger_is_better')"
```

### 2. Add New Metric

1. **Create in Django admin:** Add to `categories_metric` table
2. **Update TOML:** Add to appropriate `[[metrics]]` section
3. **Implement source:** Update source adapter to fetch the metric
4. **Use in formula:** Reference as `metric.your_slug` in score formulas

### 3. Add New Score

Add to TOML config:

```toml
[[scores]]
slug = "responsiveness"
formula = "compute_score(metric.issue_response_time, [24, 72, 168, 336], 'smaller_is_better')"
```

### 4. Conditional Logic

Use `if()` for special cases:

```toml
[[scores]]
slug = "custom_score"
formula = "if(metric.lines_of_code > 100000, 1, compute_score(metric.complexity, [5, 10, 20, 30], 'smaller_is_better'))"
```

## Benefits of TOML System

1. **Externalized Configuration** - No code changes for threshold adjustments
2. **Time-Series Storage** - Historical metrics enable re-scoring and trend analysis
3. **Conditional Logic** - Handle special cases (mirrors, large projects, etc.)
4. **Transparent Scoring** - All formulas visible in config
5. **Safe Evaluation** - No arbitrary code execution
6. **Simple Architecture** - Sequential pipeline, no complex dependencies
7. **Django Integration** - Works with existing Argus du Libre database

## Development Guide

### Adding a New Source Adapter

1. Implement `engine.SourceAdapter` interface:
   ```go
   type MySource struct{}
   func (s *MySource) Name() string { return "MySource" }
   func (s *MySource) Fetch(ctx context.Context, execCtx *engine.ExecutionContext) ([]engine.MetricResult, error)
   ```

2. Return metrics with slugs:
   ```go
   return []engine.MetricResult{
       {Slug: "my_metric", Value: 42.0, Source: "qsos-lng:mysource"},
   }
   ```

3. Register in `main.go`:
   ```go
   sourceAdapters := []engine.SourceAdapter{
       sources.NewMySource(),
       // ...
   }
   ```

### Testing Formulas

Use the formula evaluator directly:

```go
evaluator := formula.NewEvaluator(db, lookup, softwareID)
result, err := evaluator.Evaluate(ctx, "compute_score(metric.stars, [100, 1000, 10000, 100000], 'bigger_is_better')")
```

### Running Tests

```bash
go test ./pkg/...
```

## Migration Notes

- Legacy packages (`community/`, `tech/`, `security/`) preserved but not used in main pipeline
- Old `main.go` backed up (can be restored from git if needed)
- Formula system matches old hardcoded thresholds exactly
- Database schema unchanged (only adds `categories_metricvalue` usage)
