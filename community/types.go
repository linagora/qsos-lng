package community

import "time"

// CommunityData contains all data fetched from GitHub
type CommunityData struct {
	FirstCommitDate    time.Time
	LastCommitDate     time.Time
	Stars              int64
	ActiveContributors int64
	Documentation      *DocumentationData
}

// DocumentationData contains documentation-related metrics
type DocumentationData struct {
	ReadmeLength       int64 // Word count in README
	HasDocsDirectory   bool
	DocsFileCount      int64 // Number of files in docs directory
	HasWiki            bool
	HasContributing    bool
	HasCodeOfConduct   bool
	HasIssueTemplates  bool
	MultiLanguageCount int64 // Number of additional language READMEs (README.fr.md, etc.)
	KeySectionsCount   int64 // Count of key sections in README (Installation, Usage, API, Contributing, etc.)
}

// CommunityThresholds contains thresholds for all community scopes
type CommunityThresholds struct {
	Maturity      [4]int64
	Activity      [4]int64
	Popularity    [4]int64
	Contributors  [4]int64
	Documentation [4]int64
}
