package mqtt

import (
	"sync/atomic"
	"time"
)

// InboundPublishStats reports bounded queue pressure for inbound publishes.
type InboundPublishStats struct {
	QueueLen            int
	QueueCap            int
	DroppedQoS0         uint64
	EnqueueTimeoutQoS12 uint64
	DisconnectsQoS12    uint64
}

// InboundPublishStatsProvider exposes inbound publish queue diagnostics.
type InboundPublishStatsProvider interface {
	InboundPublishStats() InboundPublishStats
}

type inboundPublishLimiter struct {
	interval time.Duration
	lastLog  int64
}

const inboundPublishLogInterval = 10 * time.Second

func newInboundPublishLimiter(interval time.Duration) inboundPublishLimiter {
	return inboundPublishLimiter{interval: interval}
}

// Allow returns true when a log should be emitted.
func (l *inboundPublishLimiter) Allow(now time.Time) bool {
	if l.interval <= 0 {
		return true
	}
	nowNanos := now.UnixNano()
	last := atomic.LoadInt64(&l.lastLog)
	if nowNanos-last < l.interval.Nanoseconds() {
		return false
	}
	return atomic.CompareAndSwapInt64(&l.lastLog, last, nowNanos)
}

func (c *client) InboundPublishStats() InboundPublishStats {
	if c == nil {
		return InboundPublishStats{}
	}
	stats := InboundPublishStats{
		DroppedQoS0:         atomic.LoadUint64(&c.inboundPublishDropsQoS0),
		EnqueueTimeoutQoS12: atomic.LoadUint64(&c.inboundPublishQoS12Timeouts),
		DisconnectsQoS12:    atomic.LoadUint64(&c.inboundPublishQoS12Disconnects),
	}
	c.inboundPublishQueueMu.RLock()
	queue := c.inboundPublishQueue
	c.inboundPublishQueueMu.RUnlock()
	if queue != nil {
		stats.QueueLen = len(queue)
		stats.QueueCap = cap(queue)
	}
	return stats
}
