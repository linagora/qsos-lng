# QSOS::LNG

1. Create a [personal access token](https://github.com/settings/tokens) for the GitHub API
2. Configure with env variables:
   - `GITHUB_TOKEN` for the GitHub API token
   - `SONARQUBE_URL` for the URL of a SonarQube server
   - `SONARQUBE_TOKEN` for a token of this server
3. Run `go run . minio/minio`

## Notes

Running sonar-scanner-cli can be quite slow. It may be practical to skip this
step in development, when we already have data in SonarQube. For that, we can
use the env variable `SKIP_SONAR_SCANNER=true` when running the analyzer.
