package ui

import "io"

// Surface abstracts the dashboard/UI so alternative console renderers can plug in.
// Implementations must be safe for concurrent calls from ingest and stats loops.
type Surface interface {
	WaitReady()
	Stop()
	SetStats(lines []string)
	AppendDropped(line string)
	AppendCall(line string)
	AppendUnlicensed(line string)
	AppendHarmonic(line string)
	AppendReputation(line string)
	AppendSystem(line string)
	SystemWriter() io.Writer
	SetSnapshot(snapshot Snapshot)
}
