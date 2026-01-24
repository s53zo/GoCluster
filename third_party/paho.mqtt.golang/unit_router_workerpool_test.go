package mqtt

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"
)

func TestMatchAndDispatchWorkerPoolAllowsParallel(t *testing.T) {
	router := newRouter()
	var first int32
	started := make(chan struct{})
	block := make(chan struct{})
	processed := make(chan struct{}, 2)

	router.addRoute("a", func(c Client, m Message) {
		if atomic.CompareAndSwapInt32(&first, 0, 1) {
			close(started)
			<-block
		}
		processed <- struct{}{}
	})

	msgs := make(chan *packets.PublishPacket, 2)
	ackOut := router.matchAndDispatch(msgs, false, 2, &client{oboundP: make(chan *PacketAndToken, 10)})
	done := make(chan struct{})
	go func() {
		for range ackOut {
		}
		close(done)
	}()

	pub := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	pub.Qos = 0
	pub.TopicName = "a"
	msgs <- pub
	msgs <- pub
	close(msgs)

	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("first handler did not start")
	}

	select {
	case <-processed:
		// second message should process while first is blocked.
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected parallel handler execution with worker pool")
	}

	close(block)

	select {
	case <-processed:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected blocked handler to complete")
	}

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("matchAndDispatch did not exit")
	}
}
