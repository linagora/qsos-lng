package config

// Config represents the complete TOML configuration for QSOS-LNG
type Config struct {
	Metrics  []MetricDefinition `toml:"metrics"`
	Scores   []ScoreDefinition  `toml:"scores"`
	Metadata []MetadataOp       `toml:"metadata"`
	Output   OutputConfig       `toml:"output"`
}

// MetricDefinition defines a metric collection operation
type MetricDefinition struct {
	Name   string        `toml:"name"`   // e.g., "github_repo_info", "lizard_analysis"
	Fields []MetricField `toml:"fields"` // List of fields to extract
}

// MetricField maps an API field to a metric slug
type MetricField struct {
	APIField   string `toml:"api_field"`   // Field from the data source API
	MetricSlug string `toml:"metric_slug"` // Slug in categories_metric table
}

// ScoreDefinition defines a scoring rule using a formula
type ScoreDefinition struct {
	Slug    string `toml:"slug"`    // Score identifier (e.g., "maturity", "popularity")
	Formula string `toml:"formula"` // Formula string to evaluate
}

// MetadataOp defines a metadata enrichment operation
type MetadataOp struct {
	Name   string `toml:"name"`    // Operation name (e.g., "bilingual_summaries")
	Type   string `toml:"type"`    // Operation type: "ai_summarize", "ai_tags", "icon_url"
	SaveTo string `toml:"save_to"` // Where to save: "categories_block", "categories_tag", etc.
}

// OutputConfig controls execution flow and output settings
type OutputConfig struct {
	SaveMetrics         bool              `toml:"save_metrics"`
	MetricsTable        string            `toml:"metrics_table"`
	SaveScores          bool              `toml:"save_scores"`
	ScoreTable          string            `toml:"score_table"`
	ScoreMetadata       map[string]bool   `toml:"score_metadata"`
	UpdateSoftwareState string            `toml:"update_software_state"`
}
