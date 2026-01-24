# Upstream
- Module: github.com/eclipse/paho.mqtt.golang
- Version: v1.5.1
- Source: C:\Users\Developer\go\pkg\mod\github.com\eclipse\paho.mqtt.golang@v1.5.1
- Imported: 2026-01-23 (UTC)

## Local changes
1) Add bounded inbound publish queue with QoS0 drop / QoS1-2 timeout disconnect semantics.
2) Add fixed worker pool for dispatch when ordering is not required.
3) Add inbound queue diagnostics counters and snapshots.
