package common

type ProjectScores struct {
	Community *CommunityScores
	Tech      *TechScores
	Security  *SecurityScores
}

type CommunityScores struct {
	Maturity      int64
	Activity      int64
	Popularity    int64
	Contributors  int64
	Documentation int64
}

type TechScores struct {
	Size       int64
	Complexity int64
}

type SecurityScores struct {
	ScoreCard int64
}
