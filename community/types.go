package community

import "time"

// CommunityData contains all data fetched from GitHub
type CommunityData struct {
	FirstCommitDate    time.Time
	LastCommitDate     time.Time
	Stars              int64
	ActiveContributors int64
}

// CommunityThresholds contains thresholds for all community scopes
type CommunityThresholds struct {
	Maturity     [4]int64
	Activity     [4]int64
	Popularity   [4]int64
	Contributors [4]int64
}
