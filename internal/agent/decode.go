package agent

import (
	"bytes"
	"encoding/json"
)

// LineDecoder keeps every JSON object as an observation. A payload that claims
// to be a platform control event stays nested text and is never retyped.
func DecodeLine(stream string, frame []byte) ([]ObservedEvent, error) {
	frame = bytes.TrimSpace(frame)
	if len(frame) == 0 {
		return nil, nil
	}
	if len(frame) > 1<<20 {
		return []ObservedEvent{{Origin: "agent_observation", Type: "truncated", Payload: must(map[string]any{"stream": stream, "bytes": len(frame)})}}, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(frame, &raw); err != nil {
		return []ObservedEvent{TextEvent(stream, frame)}, nil
	}
	events := []ObservedEvent{{
		Origin:  "agent_observation",
		Type:    "agent_event",
		Payload: must(map[string]any{"stream": stream, "raw": json.RawMessage(append([]byte(nil), frame...))}),
	}}
	if usage, ok := raw["usage"]; ok {
		events = append(events, ObservedEvent{Origin: "agent_observation", Type: "usage", Payload: usage})
	}
	return events, nil
}

type lineDecoder struct{}

func (lineDecoder) Decode(stream string, frame []byte) ([]ObservedEvent, error) {
	return DecodeLine(stream, frame)
}

func NewLineDecoder() EventDecoder { return lineDecoder{} }
