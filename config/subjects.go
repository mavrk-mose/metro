package config

import "time"

// Subject hierarchy — NATS uses dot-separated subjects
// ">" is a wildcard matching one or more tokens
const (
	SubjTrainPosition = "train.position.>" // train.position.{lineId}.{trainId}
	SubjDelayEvent    = "delay.event.>"    // delay.event.{lineId}
	SubjETAUpdate     = "eta.update.%s"    // eta.update.{stationId}  (printf)
	SubjLineStatus    = "line.status.%s"   // line.status.{lineId}
)

type TrainPositionMsg struct {
	TrainID    string    `json:"train_id"`
	LineID     string    `json:"line_id"`
	CurrentStn string    `json:"current_stn"`
	NextStn    string    `json:"next_stn"`
	Progress   float64   `json:"progress"` // 0.0–1.0 along current segment
	SpeedKMH   float64   `json:"speed_kmh"`
	Crowding   int       `json:"crowding"` // 0–3
	Timestamp  time.Time `json:"ts"`
}

type DelayEvent struct {
	LineID      string        `json:"line_id"`
	FromStation string        `json:"from_station"`
	ToStation   string        `json:"to_station"`
	DelaySecs   int           `json:"delay_secs"`
	Cause       string        `json:"cause"` // "signal","mechanical","passenger"
	Severity    DelaySeverity `json:"severity"`
	DetectedAt  time.Time     `json:"detected_at"`
}

type DelaySeverity int

const (
	SeverityMinor    DelaySeverity = iota // <3 min
	SeverityModerate                      // 3–10 min
	SeverityMajor                         // >10 min, triggers full route cache bust
)

type ETAUpdateMsg struct {
	StationID string    `json:"station_id"`
	LineID    string    `json:"line_id"`
	Direction string    `json:"direction"`
	TrainID   string    `json:"train_id"`
	ETASecs   int       `json:"eta_secs"`
	Crowding  int       `json:"crowding"`
	IsDelayed bool      `json:"is_delayed"`
	UpdatedAt time.Time `json:"updated_at"`
}
