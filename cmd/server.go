package main

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	pg     *pgxpool.Pool
	rdb    *redis.Client
	nc     *nats.Conn
	graph  *SubwayGraph
	config *Config

	// WebSocket hub for live ETA updates
	wsHub *WSHub
}

func NewServer(pg *pgxpool.Pool, rdb *redis.Client, nc *nats.Conn, graph *SubwayGraph, config *Config) *Server {
	s := &Server{
		pg:    pg,
		rdb:   rdb,
		nc:    nc,
		graph: graph,
		config: config,
		wsHub: NewWSHub(),
	}

	// Start WebSocket hub
	go s.wsHub.Run()

	return s
}

func (s *Server) RegisterRoutes(r *gin.Engine) {
	// REST endpoints
	api := r.Group("/api/v1")
	{
		// Routing
		api.GET("/routes", s.HandleGetRoutes)

		// Stations
		api.GET("/stations/nearest", s.HandleNearestStations)
		api.GET("/stations/search", s.HandleSearchStations)
		api.GET("/stations/:id", s.HandleGetStation)
		api.GET("/stations/:id/arrivals", s.HandleGetArrivals)

		// Lines
		api.GET("/lines/:id/status", s.HandleGetLineStatus)
	}

	// WebSocket
	r.GET("/ws/v1/stations/:id/arrivals", s.HandleETASubscription)
}

// --- Route Handlers ---

// HandleGetRoutes computes a route between two stations
// Query params: from, to, at (optional timestamp), pref (fastest|least_transfer|least_walk)
func (s *Server) HandleGetRoutes(c *gin.Context) {
	from := c.Query("from")
	to := c.Query("to")
	pref := c.DefaultQuery("pref", "fastest")

	if from == "" || to == "" {
		c.JSON(400, ErrorResponse("from and to query params required"))
		return
	}

	// Check if we have the stations
	if _, ok := s.graph.stations[from]; !ok {
		c.JSON(404, ErrorResponse("station not found: "+from))
		return
	}
	if _, ok := s.graph.stations[to]; !ok {
		c.JSON(404, ErrorResponse("station not found: "+to))
		return
	}

	// Try to get cached route
	route, err := GetCachedRoute(c.Request.Context(), s.rdb, from, to, pref)
	if err == nil && route != nil {
		c.JSON(200, route)
		return
	}

	// Compute shortest path
	route, err := s.graph.ShortestRoute(from, to)
	if err != nil {
		c.JSON(400, ErrorResponse("no route found"))
		return
	}

	// Cache the result
	if err := CacheRoute(c.Request.Context(), s.rdb, from, to, pref, route); err != nil {
		// Log but don't fail the request
		c.Error(err)
	}

	c.JSON(200, route)
}

// HandleNearestStations returns the N closest stations to a lat/lon
// Query params: lat, lon, limit (default 3)
func (s *Server) HandleNearestStations(c *gin.Context) {
	latStr := c.Query("lat")
	lonStr := c.Query("lon")
	limitStr := c.DefaultQuery("limit", "3")

	if latStr == "" || lonStr == "" {
		c.JSON(400, ErrorResponse("lat and lon query params required"))
		return
	}

	lat, err := parseFloat(latStr)
	if err != nil {
		c.JSON(400, ErrorResponse("invalid lat"))
		return
	}

	lon, err := parseFloat(lonStr)
	if err != nil {
		c.JSON(400, ErrorResponse("invalid lon"))
		return
	}

	limit, _ := parseInt(limitStr, 3)

	// Query Postgres with PostGIS
	stations, err := s.nearestStationsFromDB(c.Request.Context(), lat, lon, limit)
	if err != nil {
		c.JSON(500, ErrorResponse("database error"))
		return
	}

	c.JSON(200, gin.H{
		"stations": stations,
	})
}

// HandleSearchStations searches stations by Korean/English name
// Query params: q, lang (ko|en)
func (s *Server) HandleSearchStations(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(400, ErrorResponse("q query param required"))
		return
	}

	// Search in-memory graph
	results := make([]*Station, 0)
	for _, stn := range s.graph.stations {
		if stringContains(stn.NameKo, q) || stringContains(stn.NameEn, q) {
			results = append(results, stn)
			if len(results) >= 10 {
				break
			}
		}
	}

	c.JSON(200, gin.H{
		"results": results,
	})
}

// HandleGetStation returns details for a single station
func (s *Server) HandleGetStation(c *gin.Context) {
	id := c.Param("id")

	stn, ok := s.graph.stations[id]
	if !ok {
		c.JSON(404, ErrorResponse("station not found"))
		return
	}

	c.JSON(200, stn)
}

// HandleGetArrivals returns next N trains arriving at a station
// Query params: station_id, line_id (optional), limit (default 5)
func (s *Server) HandleGetArrivals(c *gin.Context) {
	stnID := c.Param("id")
	limitStr := c.DefaultQuery("limit", "5")
	limit, _ := parseInt(limitStr, 5)

	// Get sorted set of arrivals from Redis
	arrivals, err := s.getArrivalsFromRedis(c.Request.Context(), stnID, limit)
	if err != nil {
		c.JSON(500, ErrorResponse("failed to fetch arrivals"))
		return
	}

	c.JSON(200, gin.H{
		"station_id": stnID,
		"arrivals":   arrivals,
	})
}

// HandleGetLineStatus returns operational status for a line
func (s *Server) HandleGetLineStatus(c *gin.Context) {
	lineID := c.Param("id")

	status, err := s.getLineStatusFromRedis(c.Request.Context(), lineID)
	if err != nil {
		c.JSON(200, gin.H{
			"line_id":    lineID,
			"disrupted":  false,
			"delay_secs": 0,
		})
		return
	}

	c.JSON(200, status)
}

// HandleETASubscription upgrades HTTP to WebSocket for live ETA updates
func (s *Server) HandleETASubscription(c *gin.Context) {
	stnID := c.Param("id")

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// In production, be more restrictive
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(400, ErrorResponse("websocket upgrade failed"))
		return
	}

	client := &WSClient{
		hub:    s.wsHub,
		conn:   conn,
		send:   make(chan interface{}, 256),
		stnID:  stnID,
		natsNC: s.nc,
	}

	s.wsHub.register <- client

	// Run read and write pumps
	go client.readPump()
	go client.writePump()
}
