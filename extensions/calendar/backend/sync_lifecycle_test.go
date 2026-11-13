package backend

import (
	"context"
	"testing"
	"time"
)

func TestSyncerStopWaitsAndRejectsNewWork(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := NewSyncer(store, nil, &recordingEventBus{}, nil, nil, nil, nil)
	s.StartContext(context.Background())
	entered := make(chan struct{})
	release := make(chan struct{})
	stopped := make(chan struct{})
	s.runWorker(func(context.Context) { close(entered); <-release })
	<-entered
	go func() { s.Stop(); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("closed while worker was running")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop did not drain")
	}
	s.runWorker(func(context.Context) { t.Error("work started after stop") })
	s.Stop()
}
