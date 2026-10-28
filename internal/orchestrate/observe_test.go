package orchestrate

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestSlogObserverEmitsStageJSON(t *testing.T) {
	var buf bytes.Buffer
	obs := SlogObserver{Logger: slog.New(slog.NewJSONHandler(&buf, nil))}
	obs.Stage(context.Background(), StageEvent{
		Stage:  StageAssess,
		Seam:   SeamDeterministic,
		Status: "ok",
		Attrs:  map[string]any{"verdict": "PASS"},
	})
	line := buf.String()
	if !strings.Contains(line, `"msg":"pathfinder.stage"`) && !strings.Contains(line, "pathfinder.stage") {
		t.Fatalf("expected pathfinder.stage log, got %s", line)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &payload); err != nil {
		t.Fatalf("log is not JSON: %v (%s)", err, line)
	}
	if payload["stage"] != StageAssess || payload["seam"] != SeamDeterministic {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestTraceStageRecordsError(t *testing.T) {
	var events []StageEvent
	obs := ObserverFunc(func(_ context.Context, e StageEvent) { events = append(events, e) })
	err := traceStage(context.Background(), obs, StageAssist, SeamLLMAssist, nil, func() error {
		return errAssistHITLTest
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if len(events) != 2 || events[0].Status != "started" || events[1].Status != "error" {
		t.Fatalf("events=%+v", events)
	}
}

var errAssistHITLTest = errString("hitl")

type errString string

func (e errString) Error() string { return string(e) }

// ObserverFunc adapts a function to Observer.
type ObserverFunc func(context.Context, StageEvent)

func (f ObserverFunc) Stage(ctx context.Context, e StageEvent) { f(ctx, e) }
