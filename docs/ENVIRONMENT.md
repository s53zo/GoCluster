## Telnet Input Guardrails

Telnet ingress now enforces strict limits to prevent memory abuse and control characters from ever reaching the command processor. Two YAML-backed knobs expose these guardrails so operators can tune them per environment:

- `telnet.login_line_limit` &mdash; defaults to `32`. This caps how many bytes a connecting client may type before the login prompt rejects the session. Keep this low so a single unauthenticated socket cannot allocate huge buffers.
- `telnet.command_line_limit` &mdash; defaults to `128`. All post-login commands (PASS/REJECT/SHOW FILTER, HELP, etc.) must fit within this byte budget. Raise the value if you run bulk filter automation that sends long comma-delimited lists.

Both limits apply before parsing; the server drops the connection and logs the offending callsign when the guardrail triggers. Commands continue to support comma-separated filter inputs, so scripted clients do not need to swap to space delimiters when the limit is increased.

## PSKReporter MQTT Debug Logging

Set `DXC_PSKR_MQTT_DEBUG=true` to enable verbose Paho MQTT debug logs for the PSKReporter client. Logs include DEBUG/WARN/ERROR/CRITICAL lines and should be used only while diagnosing reconnects or payload handling issues.
