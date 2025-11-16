package event

import (
	"errors"
	"fmt"
)

// EventBus 根据事件类型将消息路由到三个物理通道。
type EventBus struct {
	progress chan<- Event
	control  chan<- Event
	monitor  chan<- Event
}

var (
	errNilBus          = errors.New("event: bus is nil")
	errUnknownEvent    = errors.New("event: unknown type")
	errUnboundProgress = errors.New("event: progress channel not bound")
	errUnboundControl  = errors.New("event: control channel not bound")
	errUnboundMonitor  = errors.New("event: monitor channel not bound")
)

// NewEventBus 创建解耦事件总线，调用方掌控各自通道的缓冲与消费策略。
func NewEventBus(progress, control, monitor chan<- Event) *EventBus {
	return &EventBus{
		progress: progress,
		control:  control,
		monitor:  monitor,
	}
}

// Emit 根据事件类型映射到对应的通道。
func (b *EventBus) Emit(evt Event) error {
	if b == nil {
		return errNilBus
	}
	normalized := normalizeEvent(evt)
	if err := normalized.Validate(); err != nil {
		return err
	}

	ch, ok := channelForType(normalized.Type)
	if !ok {
		return fmt.Errorf("%w: %s", errUnknownEvent, normalized.Type)
	}

	switch ch {
	case ChannelProgress:
		return b.dispatch(b.progress, normalized, errUnboundProgress)
	case ChannelControl:
		return b.dispatch(b.control, normalized, errUnboundControl)
	case ChannelMonitor:
		return b.dispatch(b.monitor, normalized, errUnboundMonitor)
	default:
		return fmt.Errorf("%w: %s", errUnknownEvent, normalized.Type)
	}
}

func (b *EventBus) dispatch(ch chan<- Event, evt Event, errUnbound error) error {
	if ch == nil {
		return errUnbound
	}
	ch <- evt
	return nil
}
