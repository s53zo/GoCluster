package reputation

import "time"

// LookupResult holds normalized IP enrichment data.
type LookupResult struct {
	ASN           string
	CountryCode   string
	CountryName   string
	ContinentCode string
	Source        string
	FetchedAt     time.Time
}
