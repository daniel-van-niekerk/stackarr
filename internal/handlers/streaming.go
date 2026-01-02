package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/daniel-van-niekerk/stackarr/internal/streaming"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// StreamingHandlers holds dependencies for streaming handlers
type StreamingHandlers struct {
	ProgressManager *streaming.ProgressManager
}

// StreamProgress handles SSE progress streaming for container operations
func (h *StreamingHandlers) StreamProgress(w http.ResponseWriter, r *http.Request) {
	operationID := chi.URLParam(r, "operationID")
	if operationID == "" {
		http.Error(w, "Missing operationID", http.StatusBadRequest)
		return
	}

	log.Info().Str("operation_id", operationID).Msg("SSE client connected")

	// Subscribe to operation progress
	progressChan, err := h.ProgressManager.Subscribe(operationID)
	if err != nil {
		log.Warn().Str("operation_id", operationID).Err(err).Msg("Failed to subscribe to operation")
		sendSSEError(w, "Operation not found or already complete")
		return
	}
	defer h.ProgressManager.Unsubscribe(operationID, progressChan)

	// Check if ResponseWriter supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Error().Msg("ResponseWriter does not support Flusher")
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Initial connection established
	fmt.Fprintf(w, "data: %s\n\n", `{"type":"connected","message":"Connected to operation stream"}`)
	flusher.Flush()

	// Stream events until operation completes or client disconnects
	for {
		select {
		case event, ok := <-progressChan:
			if !ok {
				// Channel closed, operation complete
				log.Info().Str("operation_id", operationID).Msg("Operation complete, closing SSE connection")
				return
			}

			// Send event as SSE
			sendSSEEvent(w, event)
			flusher.Flush()

			// Close connection after final event
			if event.Type == "complete" || event.Type == "error" {
				log.Info().Str("operation_id", operationID).Str("event_type", event.Type).Msg("Final event sent, closing connection")
				return
			}

		case <-r.Context().Done():
			// Client disconnected
			log.Info().Str("operation_id", operationID).Msg("Client disconnected")
			return

		case <-time.After(15 * time.Minute):
			// SSE timeout
			log.Warn().Str("operation_id", operationID).Msg("SSE connection timeout")
			sendSSEError(w, "Operation timeout")
			return
		}
	}
}

// sendSSEEvent sends a progress event as Server-Sent Event format
func sendSSEEvent(w http.ResponseWriter, event streaming.ProgressEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal progress event")
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
}

// sendSSEError sends an error message in SSE format
func sendSSEError(w http.ResponseWriter, message string) {
	event := streaming.ProgressEvent{
		Type:      "error",
		Message:   message,
		Timestamp: time.Now().Format(time.RFC3339),
	}
	sendSSEEvent(w, event)
}
