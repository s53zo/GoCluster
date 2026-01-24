package mqtt

import (
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"
)

type inboundEnqueueResult int

const (
	inboundEnqueueOK inboundEnqueueResult = iota
	inboundEnqueueDroppedQoS0
	inboundEnqueueTimeoutQoS12
	inboundEnqueueCommsError
)

// enqueueInboundPublish attempts to enqueue an inbound publish while respecting queue bounds
// and QoS requirements. It returns a result and optional comms error.
func enqueueInboundPublish(queue chan *packets.PublishPacket, queueDepth int, pub *packets.PublishPacket, commsErrors <-chan error, enqueueTimeout time.Duration, timer **time.Timer) (inboundEnqueueResult, error) {
	if queueDepth == 0 {
		for {
			select {
			case queue <- pub:
				return inboundEnqueueOK, nil
			case err, ok := <-commsErrors:
				if !ok {
					commsErrors = nil
					continue
				}
				return inboundEnqueueCommsError, err
			}
		}
	}

	if pub.Qos == 0 {
		for {
			select {
			case queue <- pub:
				return inboundEnqueueOK, nil
			case err, ok := <-commsErrors:
				if !ok {
					commsErrors = nil
					continue
				}
				return inboundEnqueueCommsError, err
			default:
				return inboundEnqueueDroppedQoS0, nil
			}
		}
	}

	for {
		if enqueueTimeout <= 0 {
			return inboundEnqueueTimeoutQoS12, nil
		}
		if *timer == nil {
			*timer = time.NewTimer(enqueueTimeout)
		} else {
			if !(*timer).Stop() {
				select {
				case <-(*timer).C:
				default:
				}
			}
			(*timer).Reset(enqueueTimeout)
		}
		select {
		case queue <- pub:
			return inboundEnqueueOK, nil
		case err, ok := <-commsErrors:
			if !ok {
				commsErrors = nil
				continue
			}
			return inboundEnqueueCommsError, err
		case <-(*timer).C:
			return inboundEnqueueTimeoutQoS12, nil
		}
	}
}
