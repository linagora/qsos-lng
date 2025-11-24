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

## Notes

Running sonar-scanner-cli can be quite slow. It may be practical to skip this
step in development, when we already have data in SonarQube. For that, we can
use the env variable `SKIP_SONAR_SCANNER=true` when running the analyzer.
