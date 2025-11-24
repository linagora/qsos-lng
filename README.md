# QSOS::LNG

## Overview

QSOS::LNG analyzes Open-Source projects by collecting and scoring metrics from
multiple sources:

- **GitHub API**: Repository metadata, commit history, stars, and contributor activity
- **SonarQube**: Code quality metrics including complexity, duplication, and code smells
- **OpenSSF Scorecard**: Security best practices and vulnerability checks

The tool computes normalized scores across three main categories:

1. **Community**: Maturity, activity, popularity, and contributor engagement
2. **Technical Quality**: Code size, cyclomatic/cognitive complexity, duplication, and code smells
3. **Security**: Weighted scorecard checks for security best practices

## Usage

1. Create a [personal access token](https://github.com/settings/tokens) for the GitHub API
2. Configure with env variables:
   - `GITHUB_TOKEN` for the GitHub API token
   - `SONARQUBE_URL` for the URL of a SonarQube server
   - `SONARQUBE_TOKEN` for a token of this server
   - `AI_API_KEY` for the AI API key (used for generating summaries)
   - `AI_BASE_URL` (optional) for a custom AI API base URL
   - `AI_MODEL` (optional) for specifying a particular AI model
3. Run `go run . minio/minio`

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

1. Go to the Actions tab in your GitHub repository
2. Select "Analyze Project" workflow
3. Click "Run workflow"
4. Enter the project to analyze (e.g., `minio/minio`)
5. View results in the workflow log

## Notes

Running sonar-scanner-cli can be quite slow. It may be practical to skip this
step in development, when we already have data in SonarQube. For that, we can
use the env variable `SKIP_SONAR_SCANNER=true` when running the analyzer.
