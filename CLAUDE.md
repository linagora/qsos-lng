# QSOS::LNG

## Project Overview

QSOS::LNG is an Open-Source project analyzer that collects metrics from GitHub, SonarQube, and OpenSSF Scorecard, then computes normalized 1-5 scores across three categories: Community, Technical Quality, and Security. It operates in two modes:

- **analyze mode**: One-shot analysis that prints results to stdout
- **work mode**: Continuous background worker that processes draft projects from a PostgreSQL database (the database is shared with a Django project called Argus du Libre)

## Commands

### Running the Application

```bash
# One-shot analysis (analyze mode)
go run . analyze <owner/repo>

# Background worker for database integration (work mode)
go run . work

# Build binary
go build
```

### Environment Variables

Required for all operations:
- `GITHUB_TOKEN` - GitHub Personal Access Token (requires public_repo scope)
- `SONARQUBE_URL` - URL of SonarQube server
- `SONARQUBE_TOKEN` - SonarQube authentication token
- `AI_API_KEY` - API key for AI summary generation

Required for work mode only:
- `DATABASE_URL` - PostgreSQL connection string (e.g., `postgres://user:password@localhost:5432/dbname`)

Optional:
- `AI_BASE_URL` - Custom AI API endpoint
- `AI_MODEL` - Specific AI model to use (defaults to `gpt-oss-120b`)

## Architecture

### Package Structure

The codebase is organized by analysis category, with each package following a consistent **Fetch → Compute** pattern:

- **`community/`** - GitHub repository metadata, contributor activity, and documentation quality
- **`tech/`** - Code quality metrics from SonarQube
- **`security/`** - Security best practices from OpenSSF Scorecard
- **`metadata/`** - AI-generated project summaries (bilingual: French and English), AI-generated tags (reusing existing tags), and Icon URL resolution (simple-icons → devicons fallback)
- **`common/`** - Shared scoring logic and type definitions

### Data Flow: Fetch → Compute Pattern

Each category package implements a two-phase pipeline:

1. **Fetch phase** (`Fetch()` function):
   - Retrieves raw data from external APIs
   - Returns a `*CategoryData` struct with unprocessed metrics
   - Example: `community.Fetch()` returns `*CommunityData` with stars, commit dates, contributors

2. **Compute phase** (`ComputeAll()` function):
   - Takes raw data and thresholds as input
   - Applies `common.ComputeScore()` to normalize metrics to 1-5 scale
   - Returns `*CategoryScores` with computed scores
   - Example: `community.ComputeAll()` converts raw star count into a 1-5 popularity score

### Scoring System

All scores are normalized to a 1-5 scale using threshold-based bucketing:

- **`common.ComputeScore()`**: Core scoring function that maps raw values to 1-5 based on four thresholds
- **Direction parameter**: `BiggerIsBetter` (e.g., stars, maturity) or `SmallerIsBetter` (e.g., complexity, duplication)
- **Thresholds**: Defined in `main.go` for each metric (e.g., `communityThresholds`, `techThresholds`, `securityWeights`)

Example: If a project has 25,000 stars and popularity thresholds are [5k, 20k, 40k, 80k]:
- Stars (25,000) > threshold[2] (20,000) → Score = 4

### Security Scorecard Weighted Calculation

Security scoring differs from other categories:

- Uses **weighted average** instead of individual thresholds
- Weights defined in `securityWeights` map (1=low importance, 4=critical)
- Checks with score -1 are skipped (not applicable)
- Formula: `score = (sum of weighted_checks) / (sum of weights) / 2`
- Heavily weights packaging (4) and signed releases (4) for supply chain security

### Documentation Quality Scoring

The `community/documentation.go` module evaluates documentation quality using multiple metrics:

**Metrics collected from GitHub:**
- **README analysis**: Word count, key sections (Installation, Usage, API, Contributing, etc.)
- **Documentation directory**: Presence of `/docs` or `/doc` folder and file count
- **Multi-language support**: Additional language READMEs (README.fr.md, README.es.md, etc.)
- **Contribution accessibility**: CONTRIBUTING.md, CODE_OF_CONDUCT.md, issue templates
- **Wiki presence**: Whether the GitHub wiki is enabled

**Scoring algorithm (weighted composite):**
1. **README quality (40%)**: Length + key sections count
2. **Documentation coverage (30%)**: Docs directory presence + file count
3. **Accessibility (20%)**: Contributing guide + Code of Conduct + issue templates
4. **Multi-language support (10%)**: Number of additional language READMEs

The composite score (0-100%) is then mapped to 1-5 scale using thresholds:
- **1 (0-20%)**: No or very poor documentation
- **2 (20-40%)**: Partial or obsolete documentation
- **3 (40-60%)**: OK documentation
- **4 (60-80%)**: Full documentation with good structure
- **5 (80-100%)**: Excellent documentation with multi-language support and contribution guides

### Work Mode: Database Integration

Work mode continuously polls the database for draft projects:

1. Query `categories_software` for `state = 'draft'` projects
2. Run full analysis (fetch + compute for all categories)
3. Extract website URL from GitHub repository homepage field
4. Fetch icon URL (simple-icons → devicons fallback)
5. Generate bilingual AI summaries (French and English)
6. Generate 3-5 AI tags (prioritizing reuse of existing tags)
7. Save results to database in transaction:
   - Insert scores into `categories_analysisresult` table (mapped by field slugs)
   - Save summaries to `categories_block` table (with upsert on conflict)
   - Save tags to `categories_tag` and associate via `categories_software_tags` (with conflict handling)
   - Update `logo_url` and `website_url` fields in `categories_software`
   - Update project state to `'in_review'`
8. Sleep 3 seconds if no drafts found

### Icon Resolution Strategy

The `icon/` package implements a two-tier fallback system:

1. **Simple-icons** (primary): Fetches slugs.md from GitHub, normalizes project name for matching
   - URL format: `https://cdn.simpleicons.org/[SLUG]`
2. **Devicons** (fallback): Lists icon directories, tries exact match → language match → related icons
   - URL format: `https://cdn.jsdelivr.net/gh/devicons/devicon@latest/icons/[SLUG]/[SLUG]-original.svg`
   - Related icons: terminal/CLI/shell projects → bash icon

Name normalization removes prefixes (`the-`), suffixes (`.js`, `-py`), and special characters for fuzzy matching.

### AI Summary Generation

The `metadata/` package generates bilingual summaries:

- Fetches README content from GitHub
- Sends to AI API with locale-specific prompts (French: 3-4 sentence paragraph, English: same)
- Default model: `gpt-oss-120b` (configurable via `AI_MODEL`)
- Summaries saved to `categories_block` table with upsert logic (on conflict: update content and timestamp)

### AI Tag Generation

The `metadata/` package generates relevant tags for projects:

- Fetches README content from GitHub (same as summary generation)
- Queries database for all existing tags to promote reuse
- Sends README and existing tags to AI API with instructions to generate 3-5 tags
- AI prioritizes reusing existing tags when relevant, only creating new ones when necessary
- Tags are validated (2-8 tags, max 50 characters each) and normalized to lowercase
- Tag names are automatically converted to URL-friendly slugs
- Saves tags to `categories_tag` table using `ON CONFLICT` to handle duplicates
- Creates associations in `categories_software_tags` with duplicate prevention

### Website URL Extraction

Work mode extracts the homepage URL from GitHub repository metadata:

- Uses GitHub API's repository `Homepage` field
- Saves to `website_url` field in `categories_software` table
- Only populated if the homepage field exists and is non-empty

## Database Schema

Key tables referenced in the code:

- **`categories_software`**: Projects with `state` (draft/in_review/published), `repository_url`, `logo_url`, `website_url`
- **`categories_field`**: Field definitions with `slug` (used to map scores to database)
- **`categories_analysisresult`**: Score records with `software_id`, `field_id`, `score`, `is_published`, `is_manual`
- **`categories_block`**: Content blocks with `software_id`, `kind` (e.g., 'overview'), `locale` (fr/en), `content`
- **`categories_tag`**: Tag definitions with `name` and `slug` (both unique)
- **`categories_software_tags`**: Many-to-many relationship between software and tags with `software_id`, `tag_id` (unique constraint on pair)

## Key Implementation Details

### Threshold Configuration

All thresholds are centralized in `main.go` as package-level variables:
- Community thresholds use time durations (nanoseconds): 1 year maturity, 1 month activity; and percentage-based for documentation: [20, 40, 60, 80]
- Tech thresholds use code metrics: 1k-1M LOC for size, 1-20% for complexity
- Security uses weighted map (not thresholds): 1-4 importance scale

### Field Slug Mapping

Work mode maps computed scores to database fields using slugs:
- `maturity`, `activity`, `popularity`, `contributors`, `documentation` → Community scores
- `size`, `complexity`, `duplication`, `code-smells` → Tech scores
- `scorecard` → Security score

Field slugs must exist in `categories_field` table or scores are skipped with a warning.
