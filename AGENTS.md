# QSOS::LNG

## Project Overview

QSOS::LNG analyzes open-source projects by collecting metrics from GitHub, SonarQube, and OpenSSF Scorecard, then computing normalized 1-5 scores across Community, Technical Quality, and Security categories.

**Modes:**
- `go run . analyze <owner/repo>` - One-shot analysis to stdout
- `go run . work` - Background worker processing draft projects from PostgreSQL (shared with Django Argus du Libre)

**Environment Variables:**
- Required: `GITHUB_TOKEN`, `SONARQUBE_URL`, `SONARQUBE_TOKEN`, `AI_API_KEY`
- Work mode: `DATABASE_URL`
- Optional: `AI_BASE_URL`, `AI_MODEL` (defaults to `gpt-oss-120b`)

## Architecture

**Packages** (Fetch → Compute pattern):
- `community/` - GitHub metadata, contributors, documentation
- `tech/` - SonarQube code quality metrics
- `security/` - OpenSSF Scorecard security practices
- `metadata/` - AI summaries (bilingual FR/EN), tags, icon URLs (simple-icons → devicons fallback)
- `common/` - Shared scoring logic

**Data Flow:**
1. `Fetch()` retrieves raw data from APIs → returns `*CategoryData`
2. `ComputeAll()` applies thresholds → returns `*CategoryScores` (1-5 scale)

**Scoring:**
- `common.ComputeScore()` maps values to 1-5 via four thresholds
- Direction: `BiggerIsBetter` (stars, maturity) or `SmallerIsBetter` (complexity)
- Thresholds defined in `main.go` per metric
- Security uses weighted average: `(sum of weighted_checks) / (sum of weights) / 2`

**Documentation Scoring** (`community/documentation.go`):
- Metrics: README (word count, key sections), docs directory, multi-language READMEs, CONTRIBUTING.md, CODE_OF_CONDUCT.md, issue templates, wiki
- Weighted: README quality (40%), docs coverage (30%), accessibility (20%), multi-language (10%)
- Composite score (0-100%) mapped to 1-5 via thresholds [20, 40, 60, 80]

**Work Mode:** Polls `categories_software` for `state='draft'`, runs full analysis, saves to database:
- Scores → `categories_analysisresult` (mapped by field slugs)
- AI summaries (FR/EN) → `categories_block` (upsert)
- AI tags (3-5, reuses existing) → `categories_tag` + `categories_software_tags` (conflict-safe)
- Icons (simple-icons → devicons fallback) → `logo_url`
- Website (from GitHub homepage) → `website_url`
- State → `'in_review'`
- Sleeps 3s if no drafts

## Database Schema

**Tables:**
- `categories_software` - Projects: `state`, `repository_url`, `logo_url`, `website_url`
- `categories_field` - Field definitions with `slug` (maps scores to DB)
- `categories_analysisresult` - Scores: `software_id`, `field_id`, `score`, `is_published`, `is_manual`
- `categories_block` - Content: `software_id`, `kind`, `locale` (fr/en), `content`
- `categories_tag` - Tags: `name`, `slug` (unique)
- `categories_software_tags` - M2M: `software_id`, `tag_id` (unique pair)

**Field Slugs** (mapped to scores):
- Community: `maturity`, `activity`, `popularity`, `contributors`, `documentation`
- Tech: `size`, `complexity`, `duplication`, `code-smells`
- Security: `scorecard`
