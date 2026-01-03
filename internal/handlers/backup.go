package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/daniel-van-niekerk/stackarr/internal/backup"
	"github.com/daniel-van-niekerk/stackarr/internal/streaming"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// BackupHandlers handles backup and restore operations
type BackupHandlers struct {
	DB              *sql.DB
	ProgressManager *streaming.ProgressManager
	DataDir         string
}

// CreateBackup initiates an async backup operation
func (h *BackupHandlers) CreateBackup(w http.ResponseWriter, r *http.Request) {
	operationID := fmt.Sprintf("backup-%d", time.Now().Unix())

	// Register operation BEFORE starting async work (prevents race condition)
	h.ProgressManager.CreateOperation(operationID)

	// Start async backup in a goroutine
	go h.executeBackupCreate(operationID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"operation_id": operationID,
		"status":       "started",
	})
}

// executeBackupCreate executes the backup operation asynchronously
func (h *BackupHandlers) executeBackupCreate(operationID string) {
	defer h.ProgressManager.Complete(operationID)

	progressChan := make(chan streaming.ProgressEvent, 100)

	// Forward progress events to all subscribers
	go func() {
		for event := range progressChan {
			h.ProgressManager.Publish(operationID, event)
		}
	}()

	log.Info().Str("operation_id", operationID).Msg("Starting async backup")

	// Checkpoint WAL before backup
	progressChan <- streaming.ProgressEvent{
		Type:      "checkpoint_wal",
		Message:   "Checkpointing database...",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	if err := checkpointWAL(h.DB); err != nil {
		log.Warn().Err(err).Msg("Failed to checkpoint WAL, continuing anyway")
		// Continue - we'll include WAL files in backup
	}

	progressChan <- streaming.ProgressEvent{
		Type:      "create_archive",
		Message:   "Creating backup archive...",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Create backup
	tmpDir := os.TempDir()
	filename, err := backup.CreateBackup(h.DataDir, tmpDir, progressChan)
	close(progressChan)

	if err != nil {
		log.Error().Err(err).Str("operation_id", operationID).Msg("Backup creation failed")
		h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
			Type:      "error",
			Message:   fmt.Sprintf("Backup failed: %v", err),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	log.Info().Str("operation_id", operationID).Str("filename", filename).Msg("Backup created successfully")

	// Send completion event with download URL
	h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
		Type:      "complete",
		Message:   "Backup created successfully",
		Timestamp: time.Now().Format(time.RFC3339),
		Data: map[string]string{
			"filename":     filename,
			"download_url": "/backup/download/" + filename,
		},
	})
}

// DownloadBackupFile serves a backup file for download
func (h *BackupHandlers) DownloadBackupFile(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")

	// Validate filename to prevent directory traversal
	if err := validateBackupFilename(filename); err != nil {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	backupPath := filepath.Join(os.TempDir(), filename)

	// Check if file exists
	if _, err := os.Stat(backupPath); err != nil {
		http.Error(w, "Backup file not found", http.StatusNotFound)
		return
	}

	// Serve the file
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	file, err := os.Open(backupPath)
	if err != nil {
		log.Error().Err(err).Str("filename", filename).Msg("Failed to open backup file")
		http.Error(w, "Failed to open backup file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Copy file to response
	if _, err := io.Copy(w, file); err != nil {
		log.Error().Err(err).Str("filename", filename).Msg("Failed to write backup file")
	}

	// Delete file after serving (optional, removes temp files)
	go func() {
		time.Sleep(100 * time.Millisecond) // Give download time to complete
		if err := os.Remove(backupPath); err != nil {
			log.Warn().Err(err).Str("filename", filename).Msg("Failed to clean up backup file")
		}
	}()
}

// UploadBackup handles backup file upload
func (h *BackupHandlers) UploadBackup(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("backup_file")
	if err != nil {
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save uploaded file to temp directory
	tmpFile, err := os.CreateTemp(os.TempDir(), "backup-upload-*.zip")
	if err != nil {
		log.Error().Err(err).Msg("Failed to create temp file")
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, file); err != nil {
		log.Error().Err(err).Msg("Failed to save uploaded file")
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		os.Remove(tmpFile.Name())
		return
	}

	// Validate backup
	manifest, err := backup.ValidateBackup(tmpFile.Name())
	if err != nil {
		log.Warn().Err(err).Msg("Backup validation failed")
		http.Error(w, fmt.Sprintf("Invalid backup file: %v", err), http.StatusBadRequest)
		os.Remove(tmpFile.Name())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"path":     tmpFile.Name(),
		"filename": handler.Filename,
		"manifest": manifest,
	})
}

// RestoreBackup initiates an async restore operation
func (h *BackupHandlers) RestoreBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BackupPath string `json:"backup_path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.BackupPath == "" {
		http.Error(w, "backup_path is required", http.StatusBadRequest)
		return
	}

	operationID := fmt.Sprintf("restore-%d", time.Now().Unix())

	// Register operation BEFORE starting async work (prevents race condition)
	h.ProgressManager.CreateOperation(operationID)

	// Start async restore in a goroutine
	go h.executeBackupRestore(operationID, req.BackupPath)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"operation_id": operationID,
		"status":       "started",
	})
}

// executeBackupRestore executes the restore operation asynchronously
func (h *BackupHandlers) executeBackupRestore(operationID string, backupPath string) {
	defer h.ProgressManager.Complete(operationID)

	ctx := context.Background()
	progressChan := make(chan streaming.ProgressEvent, 100)

	// Forward progress events to all subscribers
	go func() {
		for event := range progressChan {
			h.ProgressManager.Publish(operationID, event)
		}
	}()

	log.Info().Str("operation_id", operationID).Str("backup_path", backupPath).Msg("Starting async restore")

	// Perform restore
	err := backup.RestoreBackup(ctx, backupPath, h.DataDir, h.DB, progressChan)
	close(progressChan)

	if err != nil {
		log.Error().Err(err).Str("operation_id", operationID).Msg("Restore failed")
		h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
			Type:      "error",
			Message:   fmt.Sprintf("Restore failed: %v", err),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	log.Info().Str("operation_id", operationID).Msg("Restore completed successfully")

	// Send completion event
	h.ProgressManager.Publish(operationID, streaming.ProgressEvent{
		Type:      "complete",
		Message:   "Restore completed successfully. Redirecting...",
		Timestamp: time.Now().Format(time.RFC3339),
		Data: map[string]string{
			"redirect": "/dashboard",
		},
	})
}

// validateBackupFilename validates that a backup filename is safe
// Only allows filenames matching the format: stackarr-backup-YYYY-MM-DD-HHMMSS.zip
func validateBackupFilename(filename string) error {
	matched, _ := regexp.MatchString(`^stackarr-backup-\d{4}-\d{2}-\d{2}-\d{6}\.zip$`, filename)
	if !matched {
		return fmt.Errorf("invalid filename format: %s", filename)
	}
	return nil
}

// checkpointWAL checkpoints the WAL file
// This is a helper since the database package doesn't export the method
func checkpointWAL(db *sql.DB) error {
	_, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	return err
}
