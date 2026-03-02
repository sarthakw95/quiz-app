package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusRecorderWriteTracksAndTruncates(t *testing.T) {
	base := httptest.NewRecorder()
	recorder := &statusRecorder{
		ResponseWriter: base,
		statusCode:     http.StatusOK,
		maxLogBytes:    10,
	}

	payload := []byte("abcdefghijklmnopqrstuvwxyz")
	written, err := recorder.Write(payload)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if written != len(payload) {
		t.Fatalf("written bytes = %d, want %d", written, len(payload))
	}
	if recorder.bytesWritten != len(payload) {
		t.Fatalf("bytesWritten = %d, want %d", recorder.bytesWritten, len(payload))
	}
	if recorder.logBody.Len() != 10 {
		t.Fatalf("log body length = %d, want 10", recorder.logBody.Len())
	}
	if !recorder.truncated {
		t.Fatalf("expected truncated flag to be true")
	}
}
