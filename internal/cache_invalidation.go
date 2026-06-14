package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/mavrk-moses/metro/config"
	"github.com/nats-io/nats.go"
)

// handleDelayEvent is called directly from NATS subscription (not via job channel)
// because delay events need low latency — they shouldn't queue behind position updates
func (p *ETAWorkerPool) handleDelayEvent(msg *nats.Msg) {
	ctx := context.Background()

	var event config.DelayEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		log.Printf("bad delay event: %v", err)
		return
	}

	log.Printf("delay detected: line %s between %s→%s (+%ds, %v)",
		event.LineID, event.FromStation, event.ToStation,
		event.DelaySecs, event.Severity)

	switch event.Severity {
	case config.SeverityMinor:
		p.bustStationETAs(ctx, event)
	case config.SeverityModerate:
		p.bustStationETAs(ctx, event)
		p.bustSegmentRoutes(ctx, event)
	case config.SeverityMajor:
		p.bustStationETAs(ctx, event)
		p.bustLineRoutes(ctx, event.LineID)
		p.updateLineStatus(ctx, event)
	}

	p.metrics.CacheBusts.Add(1)
}

// bustStationETAs removes arrival sorted sets for all stations
// downstream of the delay on that line segment
func (p *ETAWorkerPool) bustStationETAs(ctx context.Context, event config.DelayEvent) {
	downstream := p.graph.DownstreamStations(event.FromStation, event.LineID, 8)

	keys := make([]string, 0, len(downstream))
	for _, stnID := range downstream {
		keys = append(keys, fmt.Sprintf("arrivals:%s", stnID))
	}

	if len(keys) > 0 {
		if err := p.rdb.Del(ctx, keys...).Err(); err != nil {
			log.Printf("bust station ETAs: %v", err)
		}
	}
	log.Printf("busted ETA cache for %d downstream stations", len(keys))
}

// bustSegmentRoutes scans Redis for cached routes that pass through
// the delayed segment and deletes them
func (p *ETAWorkerPool) bustSegmentRoutes(ctx context.Context, event config.DelayEvent) {
	idxKey := fmt.Sprintf("routeidx:line:%s", event.LineID)
	keys, err := p.rdb.SMembers(ctx, idxKey).Result()
	if err != nil || len(keys) == 0 {
		return
	}

	toDelete := make([]string, 0)
	for _, routeKey := range keys {
		meta, err := p.rdb.HGet(ctx, routeKey+":meta", "stations").Result()
		if err != nil {
			continue
		}
		if segmentInRoute(meta, event.FromStation, event.ToStation) {
			toDelete = append(toDelete, routeKey, routeKey+":meta")
		}
	}

	if len(toDelete) > 0 {
		p.rdb.Del(ctx, toDelete...)
		log.Printf("busted %d segment route caches on line %s", len(toDelete)/2, event.LineID)
	}
}

// bustLineRoutes nukes ALL cached routes on a line — used for major disruptions
func (p *ETAWorkerPool) bustLineRoutes(ctx context.Context, lineID string) {
	idxKey := fmt.Sprintf("routeidx:line:%s", lineID)
	keys, err := p.rdb.SMembers(ctx, idxKey).Result()
	if err != nil || len(keys) == 0 {
		return
	}

	all := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		all = append(all, k, k+":meta")
	}
	p.rdb.Del(ctx, all...)
	p.rdb.Del(ctx, idxKey)
	log.Printf("busted ALL %d route caches for line %s", len(keys), lineID)
}

func segmentInRoute(stationsJSON, from, to string) bool {
	var stations []string
	json.Unmarshal([]byte(stationsJSON), &stations)
	for i := 0; i < len(stations)-1; i++ {
		if stations[i] == from && stations[i+1] == to {
			return true
		}
	}
	return false
}

// updateLineStatus flags the line as disrupted so the API
// can warn users before computing a route
func (p *ETAWorkerPool) updateLineStatus(ctx context.Context, event config.DelayEvent) {
	key := fmt.Sprintf("line:status:%s", event.LineID)

	p.rdb.HSet(ctx, key,
		"disrupted", "true",
		"delay_secs", strconv.Itoa(event.DelaySecs),
		"cause", event.Cause,
		"from_station", event.FromStation,
		"to_station", event.ToStation,
		"since", event.DetectedAt.Format(time.RFC3339),
	)
	p.rdb.Expire(ctx, key, 30*time.Minute)

	update, _ := json.Marshal(map[string]any{
		"type":       "line_disruption",
		"line_id":    event.LineID,
		"delay_secs": event.DelaySecs,
		"cause":      event.Cause,
	})
	p.nc.Publish(fmt.Sprintf(config.SubjLineStatus, event.LineID), update)
}
