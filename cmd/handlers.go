package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

// nearestStationsFromDB queries Postgres with PostGIS to find closest stations
func (s *Server) nearestStationsFromDB(ctx context.Context, lat, lon float64, limit int) ([]*StationDistance, error) {
	query := `
		SELECT id, name_ko, name_en,
		       ST_Y(location) AS lat,
		       ST_X(location) AS lon,
		       ST_Distance(location::geography, ST_Point($1, $2)::geography) AS distance_m
		FROM stations
		ORDER BY location <-> ST_Point($2, $1)
		LIMIT $3
	`

	rows, err := s.pg.Query(ctx, query, lon, lat, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]*StationDistance, 0, limit)
	for rows.Next() {
		var id, nameKo, nameEn string
		var sLat, sLon, distM float64
		if err := rows.Scan(&id, &nameKo, &nameEn, &sLat, &sLon, &distM); err != nil {
			return nil, err
		}
		results = append(results, &StationDistance{
			ID:       id,
			NameKo:   nameKo,
			NameEn:   nameEn,
			Lat:      sLat,
			Lon:      sLon,
			Distance: distM,
		})
	}

	return results, rows.Err()
}

// getArrivalsFromRedis fetches upcoming train arrivals from Redis sorted set
func (s *Server) getArrivalsFromRedis(ctx context.Context, stnID string, limit int) ([]ArrivalInfo, error) {
	key := fmt.Sprintf("arrivals:%s", stnID)

	now := float64(time.Now().Unix())
	results, err := s.rdb.ZRangeByScore(ctx, key, &redis.ZRangeByScore{
		Min: fmt.Sprintf("%.0f", now),
		Max: "+inf",
		Count: int64(limit),
	}).Result()

	if err == redis.Nil || len(results) == 0 {
		return []ArrivalInfo{}, nil
	}
	if err != nil {
		return nil, err
	}

	arrivals := make([]ArrivalInfo, 0, len(results))
	for _, trainID := range results {
		// Get train position to extract more info
		posKey := fmt.Sprintf("train:pos:%s", trainID)
		posData, err := s.rdb.Get(ctx, posKey).Bytes()
		if err != nil {
			continue
		}

		var pos TrainPositionMsg
		if err := json.Unmarshal(posData, &pos); err != nil {
			continue
		}

		// Get score (arrival timestamp) from sorted set
		score, err := s.rdb.ZScore(ctx, key, trainID).Result()
		if err != nil {
			continue
		}

		eta := time.Unix(int64(score), 0)
		etaSecs := int(eta.Sub(time.Now()).Seconds())
		if etaSecs < 0 {
			etaSecs = 0
		}

		arrivals = append(arrivals, ArrivalInfo{
			TrainID:   trainID,
			LineID:    pos.LineID,
			ETASecs:   etaSecs,
			Crowding:  pos.Crowding,
			IsDelayed: false,
			Timestamp: time.Now(),
		})
	}

	return arrivals, nil
}

// getLineStatusFromRedis fetches line operational status from Redis
func (s *Server) getLineStatusFromRedis(ctx context.Context, lineID string) (LineStatus, error) {
	key := fmt.Sprintf("line:status:%s", lineID)

	status := LineStatus{
		LineID:    lineID,
		Disrupted: false,
	}

	vals, err := s.rdb.HGetAll(ctx, key).Result()
	if err == redis.Nil {
		return status, nil
	}
	if err != nil {
		return status, err
	}

	if disrupted, ok := vals["disrupted"]; ok && disrupted == "true" {
		status.Disrupted = true
	}
	if delaySecs, ok := vals["delay_secs"]; ok {
		if d, err := strconv.Atoi(delaySecs); err == nil {
			status.DelaySecs = d
		}
	}
	if cause, ok := vals["cause"]; ok {
		status.Cause = cause
	}
	if fromStn, ok := vals["from_station"]; ok {
		status.FromStation = fromStn
	}
	if toStn, ok := vals["to_station"]; ok {
		status.ToStation = toStn
	}
	if since, ok := vals["since"]; ok {
		status.Since = since
	}

	return status, nil
}

// calculateFare computes the fare for a route based on Seoul's distance-based system
func (s *Server) calculateFare(ctx context.Context, stations []string) (int, error) {
	if len(stations) < 2 {
		return 0, fmt.Errorf("need at least 2 stations")
	}

	// Calculate total edges traversed
	totalDist := 0
	for i := 0; i < len(stations)-1; i++ {
		dist := s.graph.EdgeSecs(stations[i], stations[i+1])
		totalDist += dist
	}

	// Seoul fare: base 1,550 KRW up to 10km
	// Then additional charges per 5km
	// This is simplified; real fare is more complex
	baseFare := 1550
	additionalKM := (totalDist - 10) / 5
	if additionalKM > 0 {
		baseFare += additionalKM * 100
	}

	return baseFare, nil
}

// --- WebSocket Hub ---

type WSHub struct {
	clients    map[*WSClient]bool
	broadcast  chan interface{}
	register   chan *WSClient
	unregister chan *WSClient
}

type WSClient struct {
	hub    *WSHub
	conn   *websocket.Conn
	send   chan interface{}
	stnID  string
	natsNC *nats.Conn
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan interface{}, 256),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			// Subscribe to NATS ETA updates for this station
			subject := fmt.Sprintf(SubjETAUpdate, client.stnID)
			client.natsNC.Subscribe(subject, func(msg *nats.Msg) {
				var update ETAUpdateMsg
				json.Unmarshal(msg.Data, &update)
				client.send <- update
			})
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// If send channel is full, drop the message
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

func (c *WSClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var msg map[string]interface{}
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			break
		}

		// Handle ping/pong for keep-alive
		if ping, ok := msg["ping"]; ok && ping == true {
			c.send <- map[string]bool{"pong": true}
		}
	}
}

func (c *WSClient) writePump() {
	ticker := time.NewTicker(10 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

// --- Utility functions ---

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func parseInt(s string, defaultVal int) (int, error) {
	if s == "" {
		return defaultVal, nil
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal, err
	}
	return val, nil
}

func stringContains(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Asin(math.Sqrt(a))
	return earthRadiusKm * c
}

func toRad(deg float64) float64 {
	return deg * math.Pi / 180.0
}
