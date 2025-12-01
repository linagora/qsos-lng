package security

// SecurityData contains all data fetched from OpenSSF Scorecard
type SecurityData struct {
	Checks []struct {
		Name  string
		Score int64
	}
}
