## WebSocket event types

Source of truth: `pkg/events/events.go`.

### Transport envelope
WebSocket messages sent via `internal/websocket` use this envelope:

```json
{
  "type": "queue_entry_added",
  "stationId": "station-jemmal",
  "data": { "...": "..." },
  "timestamp": 1710000000
}
```

### Queue / booking / pass events
- `queue_updated`
- `queue_entry_added`
- `queue_entry_removed`
- `queue_entry_updated`
- `queue_reordered`
- `day_pass_created`
- `exit_pass_created`
- `ghost_booking_created`

### Printer events (planned/used by print job queue)
- `print_job_updated`

### Statistics events
- `statistics_update`

