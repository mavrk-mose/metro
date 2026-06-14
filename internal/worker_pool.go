package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mavrk-moses/metro/config"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

const JobQueueDepth = 512
const DelayThresholdSecs = 180

type ETAWorkerPool struct {
	nc      *nats.Conn
	rdb     *redis.Client
	graph   *SubwayGraph
	size    int
	jobs    chan *nats.Msg
	metrics *PoolMetrics
	wg      sync.WaitGroup
}

type PoolMetrics struct {
	Processed  atomic.Int64
	Errors     atomic.Int64
	CacheBusts atomic.Int64
	QueueDepth func() int
}

func NewETAWorkerPool(nc *nats.Conn, rdb *redis.Client, g *SubwayGraph, size int) *ETAWorkerPool {
	p := &ETAWorkerPool{
		nc:    nc,
		rdb:   rdb,
		graph: g,
		size:  size,
		jobs:  make(chan *nats.Msg, JobQueueDepth),
	}
	p.metrics = &PoolMetrics{
		QueueDepth: func() int { return len(p.jobs) },
	}
	return p
}

func (p *ETAWorkerPool) Start(ctx context.Context) error {
	posSub, err := p.nc.Subscribe(config.SubjTrainPosition, func(msg *nats.Msg) {
		select {
		case p.jobs <- msg:
		default:
			log.Printf("WARN: job queue full (%d), dropping message", JobQueueDepth)
			p.metrics.Errors.Add(1)
		}
	})
	if err != nil {
		return fmt.Errorf("subscribe train.position: %w", err)
	}
	posSub.SetPendingLimits(10000, 64*1024*1024)

	if _, err := p.nc.Subscribe(config.SubjDelayEvent, p.handleDelayEvent); err != nil {
		return fmt.Errorf("subscribe delay.event: %w", err)
	}

	for i := 0; i < p.size; i++ {
		p.wg.Add(1)
		go p.runWorker(ctx, i)
	}

	go func() {
		<-ctx.Done()
		posSub.Unsubscribe()
		close(p.jobs)
		p.wg.Wait()
		log.Println("ETA worker pool shut down cleanly")
	}()

	log.Printf("ETA worker pool started with %d workers", p.size)
	return nil
}

func (p *ETAWorkerPool) runWorker(ctx context.Context, id int) {
	defer p.wg.Done()

	for msg := range p.jobs {
		if ctx.Err() != nil {
			return
		}

		if err := p.processPosition(ctx, msg); err != nil {
			log.Printf("worker %d error: %v", id, err)
			p.metrics.Errors.Add(1)
		}
		p.metrics.Processed.Add(1)
	}
}

func (p *ETAWorkerPool) processPosition(ctx context.Context, msg *nats.Msg) error {
	var pos config.TrainPositionMsg
	if err := json.Unmarshal(msg.Data, &pos); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	pipe := p.rdb.Pipeline()

	posKey := fmt.Sprintf("train:pos:%s", pos.TrainID)
	pipe.Set(ctx, posKey, msg.Data, 60*time.Second)

	segSecs := p.graph.EdgeSecs(pos.CurrentStn, pos.NextStn)
	remainSecs := int((1.0 - pos.Progress) * float64(segSecs))

	arrivalTS := float64(time.Now().Unix() + int64(remainSecs))
	arrKey := fmt.Sprintf("arrivals:%s", pos.NextStn)
	pipe.ZAdd(ctx, arrKey, redis.Z{Score: arrivalTS, Member: pos.TrainID})
	pipe.Expire(ctx, arrKey, 5*time.Minute)

	if afterNext := p.graph.NextStation(pos.NextStn, pos.LineID); afterNext != "" {
		hopSecs := p.graph.EdgeSecs(pos.NextStn, afterNext)
		arrivalTS2 := arrivalTS + float64(hopSecs)
		pipe.ZAdd(ctx, fmt.Sprintf("arrivals:%s", afterNext),
			redis.Z{Score: arrivalTS2, Member: pos.TrainID})
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline: %w", err)
	}

	scheduled := p.graph.ScheduledArrival(pos.TrainID, pos.NextStn)
	if !scheduled.IsZero() {
		delaySecs := int(time.Unix(int64(arrivalTS), 0).Sub(scheduled).Seconds())
		if delaySecs > DelayThresholdSecs {
			go config.PublishDelay(p.nc, config.DelayEvent{
				LineID:      pos.LineID,
				FromStation: pos.CurrentStn,
				ToStation:   pos.NextStn,
				DelaySecs:   delaySecs,
				Severity:    classifySeverity(delaySecs),
				DetectedAt:  time.Now(),
			})
		}
	}

	return config.PublishETA(p.nc, config.ETAUpdateMsg{
		StationID: pos.NextStn,
		LineID:    pos.LineID,
		TrainID:   pos.TrainID,
		ETASecs:   remainSecs,
		Crowding:  pos.Crowding,
		UpdatedAt: time.Now(),
	})
}

func classifySeverity(delaySecs int) config.DelaySeverity {
	switch {
	case delaySecs < 180:
		return config.SeverityMinor
	case delaySecs < 600:
		return config.SeverityModerate
	default:
		return config.SeverityMajor
	}
}
