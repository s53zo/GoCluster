# Requirements Document

## Introduction

This document specifies requirements for a modern web-based UI that enables real-time configuration management of the DX Cluster Server without requiring restarts. The system currently requires manual YAML editing and server restarts for any configuration changes, which disrupts service and loses client connections. The new UI will provide a responsive, real-time interface for monitoring cluster health and modifying configuration parameters while the cluster continues operating.

## Glossary

- **DX Cluster Server**: The Go-based amateur radio spot aggregation and distribution system
- **Spot**: A report of an amateur radio station being heard on a specific frequency
- **Telnet Client**: A user connected to the cluster via telnet protocol for receiving spots
- **Configuration Hot-Reload**: The ability to apply configuration changes without restarting the server process
- **WebSocket**: A bidirectional communication protocol for real-time data exchange between browser and server
- **Ring Buffer**: The in-memory circular buffer storing recent spots
- **Deduplicator**: The component that filters duplicate spots based on time windows
- **Call Correction**: The consensus-based algorithm that fixes misreported callsigns
- **Filter**: Per-client rules determining which spots are delivered (band, mode, callsign patterns)
- **Dashboard**: The current terminal-based UI showing stats and event logs
- **Broadcast Worker**: Goroutine responsible for distributing spots to connected telnet clients
- **CTY Database**: The callsign prefix database used for geographic metadata lookups
- **RBN**: Reverse Beacon Network, a source of CW/RTTY/FT8/FT4 spots
- **PSKReporter**: An MQTT-based source of digital mode spots
- **Skew Correction**: Per-skimmer frequency adjustment factors applied to RBN spots
- **Known Calls**: The MASTER.SCP callsign database used for validation
- **FCC ULS**: Federal Communications Commission Universal Licensing System database
- **Grid Store**: SQLite database persisting callsign-to-grid mappings
- **Harmonic Detector**: Component that identifies and suppresses harmonic false spots
- **Frequency Averager**: Component that smooths CW/RTTY frequencies using consensus
- **Spotter Reliability**: Per-reporter quality weights used in call correction

## Requirements

### Requirement 1

**User Story:** As a cluster operator, I want to view real-time cluster statistics and health metrics in a web browser, so that I can monitor system performance without SSH access to the server.

#### Acceptance Criteria

1. WHEN the operator navigates to the web UI THEN the system SHALL display current uptime, memory usage, spot processing rates, and client connection counts
2. WHEN spot processing rates change THEN the system SHALL update the displayed metrics within 2 seconds without requiring page refresh
3. WHEN the system processes spots from multiple sources THEN the system SHALL display per-source ingestion rates broken down by mode (CW, RTTY, FT8, FT4)
4. WHEN deduplication or call correction events occur THEN the system SHALL display real-time event streams showing corrected calls, dropped harmonics, and unlicensed call rejections
5. WHEN telnet clients connect or disconnect THEN the system SHALL update the active client list showing callsign, connection time, and filter settings

### Requirement 2

**User Story:** As a cluster operator, I want to modify deduplication window settings through the web UI, so that I can tune duplicate filtering without restarting the cluster and disconnecting users.

#### Acceptance Criteria

1. WHEN the operator changes the primary deduplication window seconds THEN the system SHALL apply the new window to subsequent spots without dropping existing connections
2. WHEN the operator changes the secondary deduplication window seconds THEN the system SHALL rebuild the secondary deduplicator with the new window while preserving the primary deduplicator state
3. WHEN the operator toggles SNR preference settings THEN the system SHALL apply the new preference to future duplicate comparisons without affecting spots already in the ring buffer
4. WHEN invalid window values are submitted (negative numbers or non-numeric input) THEN the system SHALL reject the change and display a validation error message
5. WHEN configuration changes are applied THEN the system SHALL persist the new values to config.yaml so they survive server restarts

### Requirement 3

**User Story:** As a cluster operator, I want to adjust call correction parameters in real-time, so that I can optimize correction accuracy during contests or band openings without service interruption.

#### Acceptance Criteria

1. WHEN the operator changes min_consensus_reports THEN the system SHALL apply the new threshold to subsequent call correction decisions without clearing the correction index
2. WHEN the operator modifies recency_seconds for CW or RTTY THEN the system SHALL use the new window for future consensus calculations while preserving existing spot history
3. WHEN the operator adjusts frequency_tolerance_hz THEN the system SHALL apply the new tolerance to on-frequency grouping for subsequent spots
4. WHEN the operator changes distance model settings (morse/baudot/plain) THEN the system SHALL rebuild the distance cache and apply the new model to future corrections
5. WHEN the operator toggles call correction enabled/disabled THEN the system SHALL immediately start or stop applying corrections without restarting the spot processing pipeline

### Requirement 4

**User Story:** As a cluster operator, I want to modify telnet server settings through the web UI, so that I can adjust broadcast worker counts and queue sizes to handle traffic spikes without restarting.

#### Acceptance Criteria

1. WHEN the operator increases broadcast_workers THEN the system SHALL spawn additional worker goroutines and redistribute client shards without dropping spots
2. WHEN the operator decreases broadcast_workers THEN the system SHALL gracefully drain and stop excess workers after their current jobs complete
3. WHEN the operator changes broadcast_queue_size THEN the system SHALL create a new broadcast channel with the updated capacity and migrate pending spots
4. WHEN the operator modifies client_buffer_size THEN the system SHALL apply the new buffer size to newly connected clients while preserving existing client channels
5. WHEN the operator changes max_connections THEN the system SHALL enforce the new limit on subsequent connection attempts without disconnecting existing clients

### Requirement 5

**User Story:** As a cluster operator, I want to enable or disable spot sources (RBN, RBN Digital, PSKReporter) through the web UI, so that I can control ingestion without editing YAML files and restarting.

#### Acceptance Criteria

1. WHEN the operator disables an active RBN source THEN the system SHALL stop the client goroutine, close the telnet connection, and cease processing spots from that source
2. WHEN the operator enables a disabled RBN source THEN the system SHALL establish a new telnet connection, authenticate with the configured callsign, and begin processing spots
3. WHEN the operator disables PSKReporter THEN the system SHALL disconnect from the MQTT broker and stop all worker goroutines
4. WHEN the operator enables PSKReporter THEN the system SHALL connect to the MQTT broker, subscribe to configured topics, and start the worker pool
5. WHEN source enable/disable operations fail (network errors, authentication failures) THEN the system SHALL display error messages in the UI and log detailed diagnostics

### Requirement 6

**User Story:** As a cluster operator, I want to view and manage connected telnet clients through the web UI, so that I can monitor user activity and disconnect problematic clients without command-line tools.

#### Acceptance Criteria

1. WHEN the operator views the client list THEN the system SHALL display each client's callsign, IP address, connection duration, and active filter settings
2. WHEN the operator selects a client THEN the system SHALL display detailed statistics including spots delivered, spots dropped, and current filter configuration
3. WHEN the operator disconnects a client THEN the system SHALL close the telnet connection gracefully, send a disconnect message, and remove the client from the active list
4. WHEN the operator modifies a client's filter settings THEN the system SHALL apply the new filters immediately and notify the client via telnet
5. WHEN the operator views client drop counts THEN the system SHALL display per-client and aggregate statistics for queue drops and backpressure events

### Requirement 7

**User Story:** As a cluster operator, I want to trigger manual refreshes of external data sources (skew table, known calls, FCC ULS) through the web UI, so that I can update reference data without waiting for scheduled refreshes.

#### Acceptance Criteria

1. WHEN the operator triggers a skew table refresh THEN the system SHALL download the latest CSV from SM7IUN, convert to JSON, update the in-memory store, and display success or error status
2. WHEN the operator triggers a known calls refresh THEN the system SHALL download MASTER.SCP, update the in-memory cache, reseed the grid database, and display the new entry count
3. WHEN the operator triggers an FCC ULS refresh THEN the system SHALL download the ZIP archive, extract tables, rebuild the SQLite database, and display progress updates
4. WHEN manual refresh operations are in progress THEN the system SHALL display a progress indicator and prevent duplicate refresh requests
5. WHEN manual refreshes complete THEN the system SHALL update the UI with new record counts, file sizes, and last-updated timestamps

### Requirement 8

**User Story:** As a cluster operator, I want to adjust harmonic detection and frequency averaging parameters in real-time, so that I can fine-tune spot quality filtering during changing band conditions.

#### Acceptance Criteria

1. WHEN the operator changes harmonic recency_seconds THEN the system SHALL apply the new look-back window to subsequent harmonic detection without clearing the frequency history
2. WHEN the operator modifies max_harmonic_multiple THEN the system SHALL use the new limit for future harmonic checks
3. WHEN the operator adjusts frequency_averaging_tolerance_hz THEN the system SHALL apply the new tolerance to subsequent frequency averaging calculations
4. WHEN the operator changes frequency_averaging_min_reports THEN the system SHALL require the new minimum corroborators before applying averaged frequencies
5. WHEN the operator toggles harmonic detection enabled/disabled THEN the system SHALL immediately start or stop suppressing harmonics without restarting the spot pipeline

### Requirement 9

**User Story:** As a cluster operator, I want to modify spot policy settings (max age, frequency averaging) through the web UI, so that I can control spot freshness and quality without configuration file edits.

#### Acceptance Criteria

1. WHEN the operator changes max_age_seconds THEN the system SHALL apply the new age limit to subsequent spots entering the processing pipeline
2. WHEN the operator modifies frequency_averaging_seconds THEN the system SHALL use the new window for future frequency averaging calculations
3. WHEN the operator adjusts frequency_averaging_tolerance_hz THEN the system SHALL apply the new tolerance to on-frequency grouping for averaging
4. WHEN the operator changes frequency_averaging_min_reports THEN the system SHALL require the new minimum before applying averaged frequencies
5. WHEN invalid values are submitted (negative numbers, zero for required positive values) THEN the system SHALL reject the change and display validation errors

### Requirement 10

**User Story:** As a cluster operator, I want to view the current configuration state in the web UI, so that I can verify active settings and compare them to the persisted config.yaml file.

#### Acceptance Criteria

1. WHEN the operator views the configuration page THEN the system SHALL display all current runtime configuration values organized by category (dedup, call correction, telnet, sources)
2. WHEN configuration values differ between runtime and config.yaml THEN the system SHALL highlight the differences and indicate which values are active
3. WHEN the operator requests a configuration export THEN the system SHALL generate a downloadable YAML file containing current runtime settings
4. WHEN the operator uploads a configuration file THEN the system SHALL validate the YAML structure, display a diff preview, and allow selective application of changes
5. WHEN configuration validation fails THEN the system SHALL display specific error messages indicating which fields are invalid and why

### Requirement 11

**User Story:** As a cluster operator, I want the web UI to authenticate users, so that only authorized operators can modify cluster configuration.

#### Acceptance Criteria

1. WHEN an unauthenticated user accesses the web UI THEN the system SHALL display a login page requiring username and password
2. WHEN a user submits valid credentials THEN the system SHALL create a session token, store it in a secure HTTP-only cookie, and redirect to the dashboard
3. WHEN a user submits invalid credentials THEN the system SHALL display an error message and log the failed attempt with IP address and timestamp
4. WHEN an authenticated session expires (30 minutes of inactivity) THEN the system SHALL require re-authentication before allowing configuration changes
5. WHEN a user logs out THEN the system SHALL invalidate the session token and redirect to the login page

### Requirement 12

**User Story:** As a cluster operator, I want the web UI to use WebSocket connections for real-time updates, so that I see live data without polling or page refreshes.

#### Acceptance Criteria

1. WHEN the operator opens the dashboard THEN the system SHALL establish a WebSocket connection and begin streaming stats updates every 2 seconds
2. WHEN spot processing events occur (call corrections, harmonics, unlicensed drops) THEN the system SHALL push event messages through the WebSocket within 100 milliseconds
3. WHEN the WebSocket connection drops THEN the system SHALL attempt automatic reconnection with exponential backoff up to 30 seconds
4. WHEN the WebSocket reconnects THEN the system SHALL resynchronize the UI state by requesting current stats and configuration
5. WHEN the operator closes the browser tab THEN the system SHALL detect the WebSocket closure and clean up server-side resources

### Requirement 13

**User Story:** As a cluster operator, I want to view historical spot processing metrics in the web UI, so that I can analyze trends and identify performance issues over time.

#### Acceptance Criteria

1. WHEN the operator views the metrics page THEN the system SHALL display time-series charts for spot ingestion rates, deduplication ratios, and call correction rates over the past 24 hours
2. WHEN the operator selects a time range THEN the system SHALL update the charts to show data for the selected period (1 hour, 6 hours, 24 hours, 7 days)
3. WHEN the operator hovers over chart data points THEN the system SHALL display tooltips with exact values and timestamps
4. WHEN the operator exports metrics THEN the system SHALL generate a CSV file containing the selected time range data
5. WHEN metrics data is unavailable for a time range THEN the system SHALL display a message indicating the data retention period and available ranges

### Requirement 14

**User Story:** As a cluster operator, I want to configure adaptive parameters (min_reports, refresh intervals) through the web UI, so that I can tune dynamic behavior without editing nested YAML structures.

#### Acceptance Criteria

1. WHEN the operator modifies adaptive_min_reports band group thresholds THEN the system SHALL update the in-memory configuration and apply new thresholds to subsequent evaluations
2. WHEN the operator changes adaptive_refresh timing parameters THEN the system SHALL adjust the evaluation period and refresh intervals without restarting the refresher goroutine
3. WHEN the operator modifies adaptive_refresh_by_band settings THEN the system SHALL update the band-based refresh cadence for trust/quality sets
4. WHEN the operator adds or removes band groups THEN the system SHALL validate band names against supported bands and update the adaptive min_reports configuration
5. WHEN invalid adaptive configuration is submitted (negative thresholds, unknown bands) THEN the system SHALL reject the change and display validation errors

### Requirement 15

**User Story:** As a cluster operator, I want to view real-time spot streams in the web UI, so that I can monitor what spots are being processed and delivered to clients.

#### Acceptance Criteria

1. WHEN the operator views the spot stream page THEN the system SHALL display the most recent 100 spots in DX cluster format with timestamps
2. WHEN new spots are processed THEN the system SHALL push them to the web UI via WebSocket and prepend them to the stream
3. WHEN the operator applies filters (band, mode, callsign pattern) THEN the system SHALL display only spots matching the filter criteria
4. WHEN the operator pauses the stream THEN the system SHALL buffer incoming spots and resume display when unpaused
5. WHEN the operator exports the visible spot stream THEN the system SHALL generate a downloadable text file in DX cluster format
