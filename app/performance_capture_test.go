package app

import (
	"context"
	"testing"
)

func TestPerformanceCapturePropagatesClientBootstrapFailure(t *testing.T) {
	// No keyring/client means failure happens before transport instrumentation.
	connector := &mailboxServiceConnector{}
	if err := connector.performanceCaptureHandshake(context.Background()); err == nil {
		t.Fatal("capture activation silently ignored missing client authorization")
	}
}
