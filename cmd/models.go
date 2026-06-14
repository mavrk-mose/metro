package main

import (
	"time"
)

// --- API Response Models ---

type ErrorResp struct {
	Error string `json:"error"`
}

func ErrorResponse(msg string) ErrorResp {
	return ErrorResp{Error: msg}
}

// StationDistance represents a station with distance from query point
type StationDistance struct {
	ID       string  `json:"id"`
	NameKo   string  `json:"name_ko"`
	NameEn   string  `json:"name_en"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Distance float64 `json:"distance_m"`
}

// ArrivalInfo represents a single train arrival
type ArrivalInfo struct {
	TrainID   string    `json:"train_id"`
	LineID    string    `json:"line_id"`
	ETASecs   int       `json:"eta_secs"`
	Crowding  int       `json:"crowding"` // 0-3
	IsDelayed bool      `json:"is_delayed"`
	Timestamp time.Time `json:"timestamp"`
}

// LineStatus represents operational status of a line
type LineStatus struct {
	LineID      string `json:"line_id"`
	Disrupted   bool   `json:"disrupted"`
	DelaySecs   int    `json:"delay_secs"`
	Cause       string `json:"cause,omitempty"`
	FromStation string `json:"from_station,omitempty"`
	ToStation   string `json:"to_station,omitempty"`
	Since       string `json:"since,omitempty"`
}

// --- Internal Models (from worker_pool.go) ---

type TrainPositionMsg struct {
	TrainID    string    `json:"train_id"`
	LineID     string    `json:"line_id"`
	CurrentStn string    `json:"current_stn"`
	NextStn    string    `json:"next_stn"`
	Progress   float64   `json:"progress"`
	SpeedKMH   float64   `json:"speed_kmh"`
	Crowding   int       `json:"crowding"`
	Timestamp  time.Time `json:"ts"`
}

type DelayEvent struct {
	LineID      string        `json:"line_id"`
	FromStation string        `json:"from_station"`
	ToStation   string        `json:"to_station"`
	DelaySecs   int           `json:"delay_secs"`
	Cause       string        `json:"cause"`
	Severity    DelaySeverity `json:"severity"`
	DetectedAt  time.Time     `json:"detected_at"`
}

type DelaySeverity int

const (
	SeverityMinor    DelaySeverity = iota
	SeverityModerate
	SeverityMajor
)

type ETAUpdateMsg struct {
	StationID  string    `json:"station_id"`
	LineID     string    `json:"line_id"`
	Direction  string    `json:"direction,omitempty"`
	TrainID    string    `json:"train_id"`
	ETASecs    int       `json:"eta_secs"`
	Crowding   int       `json:"crowding"`
	IsDelayed  bool      `json:"is_delayed"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// --- NATS Subject Constants ---

const (
	SubjTrainPosition = "train.position.>"
	SubjDelayEvent    = "delay.event.>"
	SubjETAUpdate     = "eta.update.%s"
	SubjLineStatus    = "line.status.%s"
)
