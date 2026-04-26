package websocket

// Broadcaster is the minimal interface services need to emit realtime events.
// Hub implements it for in-process broadcasts; RedisBroadcaster implements it for cross-process fanout.
type Broadcaster interface {
	BroadcastToStation(stationID string, messageType string, data interface{})
}

