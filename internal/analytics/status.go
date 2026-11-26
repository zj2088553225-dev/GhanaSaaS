package analytics

import "time"

type DataStatus struct {
	TotalEvents   int64            `json:"total_events"`
	HumanEvents   int64            `json:"human_events"`
	BotEvents     int64            `json:"bot_events"`
	OldestEventAt *time.Time       `json:"oldest_event_at"`
	LatestEventAt *time.Time       `json:"latest_event_at"`
	EventsLast24h int64            `json:"events_last_24h"`
	EventsByName  map[string]int64 `json:"events_by_name"`
}
