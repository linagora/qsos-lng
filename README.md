# QSOS::LNG

## Overview

QSOS::LNG analyzes Open-Source projects by collecting and scoring metrics from
multiple sources:

- **GitHub API**: Repository metadata, commit history, stars, and contributor activity
- **Tokei + Lizard**: Code quality metrics (Tokei for fast line counting, Lizard for cyclomatic complexity)
- **OpenSSF Scorecard**: Security best practices and vulnerability checks

The tool computes normalized scores across three main categories:

1. **Community**: Maturity, activity, popularity, contributor engagement, and documentation quality
2. **Technical Quality**: Code size and cyclomatic complexity (percentage of high-complexity functions)
3. **Security**: Weighted scorecard checks for security best practices

## Usage

1. Create a [personal access token](https://github.com/settings/tokens) for the GitHub API
2. Configure with env variables:
   - `DATABASE_URL` for the PostgreSQL database connection string (e.g., `postgres://user:password@localhost:5432/dbname`)
   - `GITHUB_TOKEN` for the GitHub API token
   - `AI_API_KEY` for the AI API key (used for generating summaries)
   - `AI_BASE_URL` (optional) for a custom AI API base URL
   - `AI_MODEL` (optional) for specifying a particular AI model
3. Run:
   - `go run . analyze minio/minio` for one-shot analysis of a project
   - `go run . work` for background worker processing draft projects
   - `go run . retag` to regenerate AI tags on all published projects
- `go run . reicon` to regenerate icons on all published projects

## GitHub Actions Workflow

A workflow is available at `.github/workflows/analyze.yml` that can be triggered manually to analyze any project.

**Required Secrets:**

Configure these in your repository settings (Settings → Secrets and variables → Actions):

- `ANALYSIS_GITHUB_TOKEN` - A Personal Access Token with `public_repo` scope (or `repo` for private repos)
  - Required for analyzing external repositories
  - The default `GITHUB_TOKEN` has insufficient permissions for OpenSSF Scorecard checks on external repositories
  - Create one at https://github.com/settings/tokens
- `AI_API_KEY` - Your AI API key for generating summaries
- `AI_BASE_URL` (optional) - Custom AI API URL
- `AI_MODEL` (optional) - Specific AI model name

**To run the workflow:**

1. Go to the Actions tab in your GitHub repository: https://github.com/linagora/qsos-lng/actions/workflows/analyze.yml
2. Select "Analyze Project" workflow
3. Click "Run workflow"
4. Enter the project to analyze (e.g., `minio/minio`)
5. View results in the workflow log

