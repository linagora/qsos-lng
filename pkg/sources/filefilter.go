package sources

import "strings"

// skipPathPatterns contains directory/path patterns to skip for code analysis.
// Used by both Lizard (complexity analysis) and LLM debt (tree filtering).
var skipPathPatterns = []string{
	// Third-party and vendored code
	"/vendor/", "/third_party/", "/third-party/", "/thirdparty/",
	"/external/", "/extern/", "/deps/", "/dependencies/",
	"/node_modules/", "/bower_components/",
	"/contrib/", "/contributed/",

	// Build, CI, and tooling
	"/.ci/", "/.github/", "/.gitlab/", "/.circleci/",
	"/build/", "/cmake/", "/make/", "/m4/",
	"/scripts/", "/tools/", "/tooling/",
	"/.buildkite/", "/.travis/",

	// Configuration and metadata
	"/config/", "/configs/", "/configuration/",
	"/.config/", "/settings/",

	// Documentation and examples
	"/docs/", "/doc/", "/documentation/",
	"/examples/", "/example/", "/samples/", "/sample/",
	"/tutorials/", "/tutorial/", "/demo/", "/demos/",

	// Benchmarks and performance tests
	"/benchmarks/", "/benchmark/", "/bench/",
	"/perf/", "/performance/",

	// Generated code markers
	"/generated/", "/gen/", "/auto/", "/autogen/",
	"/_generated/", "/codegen/",

	// Assets and resources
	"/assets/", "/static/", "/public/",
	"/resources/", "/res/", "/data/",
	"/images/", "/img/", "/icons/",
	"/fonts/", "/locales/", "/i18n/", "/l10n/",

	// IDE and editor
	"/.idea/", "/.vscode/", "/.vs/",

	// Migrations
	"/migrations/", "/migrate/",
}

// skipExtensions contains file extensions to exclude from LLM tree prompts.
// These are non-code files that waste tokens without providing useful signal.
var skipExtensions = []string{
	// Images and media
	".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".webp", ".bmp",
	".mp3", ".mp4", ".wav", ".avi", ".mov",
	// Fonts
	".woff", ".woff2", ".ttf", ".eot", ".otf",
	// Archives and binaries
	".zip", ".tar", ".gz", ".bz2", ".xz", ".jar", ".war", ".so", ".dll", ".exe",
	// Data files
	".csv", ".tsv", ".parquet", ".sqlite", ".db",
	// Lock files
	".lock",
	// Compiled / minified
	".min.js", ".min.css", ".map",
	// Misc
	".pdf", ".psd", ".ai",
}

// shouldSkipPath checks if a file should be skipped based on directory patterns.
// The path is matched case-insensitively. A leading "/" is prepended so that
// patterns like "/vendor/" match both "vendor/foo.go" and "src/vendor/foo.go".
func shouldSkipPath(path string) bool {
	lowerPath := "/" + strings.ToLower(path)
	for _, pattern := range skipPathPatterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}
	return false
}

// shouldSkipExtension checks if a file should be skipped based on its extension.
func shouldSkipExtension(path string) bool {
	lowerPath := strings.ToLower(path)
	for _, ext := range skipExtensions {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	return false
}

// isTestFile checks if a file path represents a test file.
func isTestFile(filepath string) bool {
	filepath = strings.ReplaceAll(filepath, "\\", "/")
	filepath = strings.ToLower(filepath)

	// Common test patterns
	if strings.Contains(filepath, "_test") || strings.Contains(filepath, "test_") {
		return true
	}
	if strings.Contains(filepath, ".test.") || strings.Contains(filepath, ".spec.") {
		return true
	}

	// Test directories
	testDirs := []string{"/test/", "/tests/", "/__tests__/", "/t/", "specs", "src/test/"}
	for _, dir := range testDirs {
		if strings.Contains(filepath, dir) {
			return true
		}
	}

	return false
}
