package internal

import "time"

type Station struct {
	ID         string
	NameKo     string
	NameEn     string
	Lat        float64
	Lon        float64
	IsTransfer bool
	ZoneID     string
}

type Edge struct {
	From        string
	To          string
	Line        string
	TravelSecs  int
	IsTransfer  bool
	TransferSecs int
}

type ScheduleEntry struct {
	Direction string
	Time      time.Time
}

type ScheduleMap map[string][]ScheduleEntry

type Route struct {
	Stations []string `json:"stations"`
	Lines    []string `json:"lines"`
}

func extractStationIDs(route Route) []string {
	return route.Stations
}

func extractLineIDs(route Route) []string {
	return route.Lines
}

type SubwayGraph struct {
	schedules ScheduleMap
}

func BuildSubwayGraph(_ []Station, _ []Edge) *SubwayGraph {
	return &SubwayGraph{schedules: make(ScheduleMap)}
}

func (g *SubwayGraph) EdgeSecs(from, to string) int {
	_ = from
	_ = to
	return 60
}

func (g *SubwayGraph) NextStation(stationID, lineID string) string {
	_ = stationID
	_ = lineID
	return ""
}

func (g *SubwayGraph) ScheduledArrival(trainID, stationID string) time.Time {
	_ = trainID
	_ = stationID
	return time.Time{}
}

func (g *SubwayGraph) DownstreamStations(stationID, lineID string, max int) []string {
	_ = stationID
	_ = lineID
	_ = max
	return nil
}
