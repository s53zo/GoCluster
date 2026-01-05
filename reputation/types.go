package reputation

import "time"

// DropReason describes why a spot was rejected by the reputation gate.
type DropReason string

const (
	DropProbation DropReason = "probation"
	DropBandCap   DropReason = "band_cap"
	DropTotalCap  DropReason = "total_cap"
	DropPrefixCap DropReason = "prefix_cap"
)

// PenaltyFlags capture non-dropping conditions that influence ramp behavior.
type PenaltyFlags uint8

const (
	PenaltyCountryMismatch PenaltyFlags = 1 << iota
	PenaltyASNReset
	PenaltyGeoFlip
	PenaltyDisagreement
	PenaltyUnknown
)

// Has reports whether the provided flag is set.
func (f PenaltyFlags) Has(flag PenaltyFlags) bool {
	return f&flag != 0
}

// Request describes a single spot submission that needs a reputation decision.
type Request struct {
	Call string
	Band string
	IP   string
	Now  time.Time
}

// Decision captures the output of a gate check.
type Decision struct {
	Allow       bool
	Drop        bool
	Reason      DropReason
	Flags       PenaltyFlags
	ASN         string
	CountryCode string
	CountryName string
	Source      string
	Prefix      string
}

// DropEvent captures the data needed for logging or counters.
type DropEvent struct {
	Call        string
	Band        string
	IP          string
	Prefix      string
	Reason      DropReason
	Flags       PenaltyFlags
	ASN         string
	CountryCode string
	CountryName string
	Source      string
	When        time.Time
}
