package ui

import "time"

// Snapshot is a structured UI snapshot built by the main stats loop.
// It is immutable once handed to a Surface.
type Snapshot struct {
	GeneratedAt   time.Time
	OverviewLines []string
	IngestLines   []string
	PipelineLines []string
	NetworkLines  []string
}
