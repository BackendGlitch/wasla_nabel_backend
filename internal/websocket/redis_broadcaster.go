package websocket

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultRedisWSChannel = "wasla:ws:events"

type pubSubMessage struct {
	Type      string          `json:"type"`
	StationID string          `json:"stationId"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp"`
}

// RedisBroadcaster publishes websocket events into Redis PubSub so they can be fanned out
// by the websocket-hub process to all connected clients across the LAN.
type RedisBroadcaster struct {
	rdb     *redis.Client
	channel string
}

func NewRedisBroadcaster(rdb *redis.Client, channel string) *RedisBroadcaster {
	if strings.TrimSpace(channel) == "" {
		channel = defaultRedisWSChannel
	}
	return &RedisBroadcaster{rdb: rdb, channel: channel}
}

func NewRedisBroadcasterFromEnv() *RedisBroadcaster {
	redisURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if redisURL == "" {
		return nil
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("RedisBroadcaster: invalid REDIS_URL: %v", err)
		return nil
	}
	rdb := redis.NewClient(opt)
	channel := strings.TrimSpace(os.Getenv("WS_REDIS_CHANNEL"))
	return NewRedisBroadcaster(rdb, channel)
}

func (b *RedisBroadcaster) BroadcastToStation(stationID string, messageType string, data interface{}) {
	if b == nil || b.rdb == nil {
		return
	}
	raw, err := json.Marshal(data)
	if err != nil {
		log.Printf("RedisBroadcaster: marshal data failed: %v", err)
		return
	}

	msg := pubSubMessage{
		Type:      messageType,
		StationID: stationID,
		Data:      raw,
		Timestamp: time.Now().Unix(),
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("RedisBroadcaster: marshal message failed: %v", err)
		return
	}

	// Best-effort publish; realtime is not transactional.
	_ = b.rdb.Publish(context.Background(), b.channel, payload).Err()
}

