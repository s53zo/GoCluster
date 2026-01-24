package mqtt

import (
	"testing"
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"
)

func TestEnqueueInboundPublishDropsQoS0WhenFull(t *testing.T) {
	queue := make(chan *packets.PublishPacket, 1)
	queue <- &packets.PublishPacket{FixedHeader: packets.FixedHeader{Qos: 0}}
	msg := &packets.PublishPacket{FixedHeader: packets.FixedHeader{Qos: 0}}
	var timer *time.Timer

	result, err := enqueueInboundPublish(queue, cap(queue), msg, nil, 0, &timer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != inboundEnqueueDroppedQoS0 {
		t.Fatalf("expected QoS0 drop, got %v", result)
	}
}

func TestEnqueueInboundPublishTimesOutQoS12WhenFull(t *testing.T) {
	queue := make(chan *packets.PublishPacket, 1)
	queue <- &packets.PublishPacket{FixedHeader: packets.FixedHeader{Qos: 1}}
	msg := &packets.PublishPacket{FixedHeader: packets.FixedHeader{Qos: 1}}
	var timer *time.Timer

	result, err := enqueueInboundPublish(queue, cap(queue), msg, nil, 5*time.Millisecond, &timer)
	if timer != nil {
		timer.Stop()
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != inboundEnqueueTimeoutQoS12 {
		t.Fatalf("expected QoS1/2 timeout, got %v", result)
	}
}

func TestEnqueueInboundPublishOkWhenSpace(t *testing.T) {
	queue := make(chan *packets.PublishPacket, 1)
	msg := &packets.PublishPacket{FixedHeader: packets.FixedHeader{Qos: 0}}
	var timer *time.Timer

	result, err := enqueueInboundPublish(queue, cap(queue), msg, nil, 0, &timer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != inboundEnqueueOK {
		t.Fatalf("expected enqueue ok, got %v", result)
	}
	if len(queue) != 1 {
		t.Fatalf("expected queue length 1, got %d", len(queue))
	}
}
