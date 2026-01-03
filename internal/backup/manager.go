package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/daniel-van-niekerk/stackarr/internal/docker"
	"github.com/daniel-van-niekerk/stackarr/internal/streaming"
	"github.com/rs/zerolog/log"
)

// Manifest represents backup metadata
type Manifest struct {
	Version       string    `json:"version"`
	Timestamp     time.Time `json:"timestamp"`
	DatabaseFiles []string  `json:"database_files"`
	ComposeFiles  []string  `json:"compose_files"`
	FileCount     int       `json:"file_count"`
}

// CreateBackup creates a backup of the data directory
// Returns the filename (not full path) of the created backup
func CreateBackup(dataDir string, outputDir string, progressChan chan<- streaming.ProgressEvent) (string, error) {
	// Validate inputs
	if _, err := os.Stat(dataDir); err != nil {
		return "", fmt.Errorf("data directory does not exist: %w", err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("2006-01-02-150405")
	backupFilename := fmt.Sprintf("stackarr-backup-%s.zip", timestamp)
	backupPath := filepath.Join(outputDir, backupFilename)

	// Create the ZIP archive
	zipFile, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	manifest := &Manifest{
		Version:        "1.0",
		Timestamp:      time.Now(),
		DatabaseFiles:  []string{},
		ComposeFiles:   []string{},
		FileCount:      0,
	}

	// Add database files
	databaseFiles := []string{"stackarr.db", "stackarr.db-shm", "stackarr.db-wal"}
	for _, dbFile := range databaseFiles {
		dbPath := filepath.Join(dataDir, dbFile)

		// Check if file exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			// File doesn't exist, skip it (especially for shm and wal which may not always exist)
			log.Warn().Str("file", dbFile).Msg("Database file not found in backup, skipping")
			continue
		} else if err != nil {
			return "", fmt.Errorf("failed to stat database file %s: %w", dbFile, err)
		}

		if err := addFileToZip(zipWriter, dbPath, dbFile); err != nil {
			return "", fmt.Errorf("failed to add database file to backup: %w", err)
		}

		manifest.DatabaseFiles = append(manifest.DatabaseFiles, dbFile)
		manifest.FileCount++

		// Publish progress
		progressChan <- streaming.ProgressEvent{
			Type:      "add_file",
			Message:   fmt.Sprintf("Adding %s to backup...", dbFile),
			Timestamp: time.Now().Format(time.RFC3339),
			Data: map[string]interface{}{
				"current":  manifest.FileCount,
				"filename": dbFile,
			},
		}
	}

	// Add compose files
	composeDir := filepath.Join(dataDir, "compose")
	if _, err := os.Stat(composeDir); err == nil {
		// Compose directory exists, add all YAML files
		composeDirEntries, err := os.ReadDir(composeDir)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to read compose directory, skipping compose files")
		} else {
			for _, entry := range composeDirEntries {
				if !entry.IsDir() {
					composeFilePath := filepath.Join(composeDir, entry.Name())
					zipPath := filepath.Join("compose", entry.Name())

					if err := addFileToZip(zipWriter, composeFilePath, zipPath); err != nil {
						return "", fmt.Errorf("failed to add compose file to backup: %w", err)
					}

					manifest.ComposeFiles = append(manifest.ComposeFiles, entry.Name())
					manifest.FileCount++

					// Publish progress
					progressChan <- streaming.ProgressEvent{
						Type:      "add_file",
						Message:   fmt.Sprintf("Adding compose/%s to backup...", entry.Name()),
						Timestamp: time.Now().Format(time.RFC3339),
						Data: map[string]interface{}{
							"current":  manifest.FileCount,
							"filename": entry.Name(),
						},
					}
				}
			}
		}
	}

	// Create and add manifest file
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal manifest: %w", err)
	}

	manifestWriter, err := zipWriter.Create("manifest.json")
	if err != nil {
		return "", fmt.Errorf("failed to create manifest entry in zip: %w", err)
	}

	if _, err := manifestWriter.Write(manifestData); err != nil {
		return "", fmt.Errorf("failed to write manifest to zip: %w", err)
	}

	log.Info().
		Str("filename", backupFilename).
		Int("file_count", manifest.FileCount).
		Msg("Backup created successfully")

	return backupFilename, nil
}

// RestoreBackup restores from a backup file
func RestoreBackup(ctx context.Context, archivePath string, dataDir string, db *sql.DB, progressChan chan<- streaming.ProgressEvent) error {
	// Validate backup first
	_, err := ValidateBackup(archivePath)
	if err != nil {
		return fmt.Errorf("backup validation failed: %w", err)
	}

	progressChan <- streaming.ProgressEvent{
		Type:      "validate",
		Message:   "Backup archive validated",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Close database connection before swapping files
	if db != nil {
		progressChan <- streaming.ProgressEvent{
			Type:      "checkpoint_wal",
			Message:   "Flushing database writes...",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		// Checkpoint WAL to flush all pending writes
		if err := checkpointWAL(db); err != nil {
			log.Warn().Err(err).Msg("Failed to checkpoint WAL before restore, continuing anyway")
		}

		progressChan <- streaming.ProgressEvent{
			Type:      "close_database",
			Message:   "Closing database connection...",
			Timestamp: time.Now().Format(time.RFC3339),
		}
		db.Close()
		// Give the OS time to release file locks
		time.Sleep(200 * time.Millisecond)
	}

	// Create pre-restore backup
	tmpDir := os.TempDir()
	preRestoreBackup, err := CreateBackup(dataDir, tmpDir, progressChan)
	if err != nil {
		return fmt.Errorf("failed to create pre-restore backup: %w", err)
	}

	preRestoreBackupPath := filepath.Join(tmpDir, preRestoreBackup)

	progressChan <- streaming.ProgressEvent{
		Type:      "safety_backup",
		Message:   fmt.Sprintf("Created safety backup: %s", preRestoreBackupPath),
		Timestamp: time.Now().Format(time.RFC3339),
		Data: map[string]string{
			"backup_path": preRestoreBackupPath,
		},
	}

	// Stop all managed containers
	client, err := docker.NewClient()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create Docker client for stopping containers")
		// Continue anyway - containers might not be running or Docker might not be available
	} else {
		defer client.Close()

		containers, err := client.ListContainers(ctx, true)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to list containers for stopping")
			// Continue anyway
		} else if len(containers) > 0 {
			progressChan <- streaming.ProgressEvent{
				Type:      "stop_containers",
				Message:   fmt.Sprintf("Stopping %d containers...", len(containers)),
				Timestamp: time.Now().Format(time.RFC3339),
				Data: map[string]interface{}{
					"total": len(containers),
				},
			}

			for i, container := range containers {
				if err := client.StopContainer(ctx, container.ID); err != nil {
					log.Warn().Err(err).Str("container_id", container.ID).Msg("Failed to stop container")
					// Continue anyway - we still want to restore
				}

				progressChan <- streaming.ProgressEvent{
					Type:      "stop_containers",
					Message:   fmt.Sprintf("Stopped container %d of %d", i+1, len(containers)),
					Timestamp: time.Now().Format(time.RFC3339),
					Data: map[string]interface{}{
						"current": i + 1,
						"total":   len(containers),
					},
				}
			}
		}
	}

	// Extract to temporary directory
	tempRestoreDir := filepath.Join(os.TempDir(), fmt.Sprintf("stackarr-restore-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tempRestoreDir, 0755); err != nil {
		return fmt.Errorf("failed to create temporary restore directory: %w", err)
	}
	defer os.RemoveAll(tempRestoreDir)

	if err := extractBackupArchive(archivePath, tempRestoreDir, progressChan); err != nil {
		return fmt.Errorf("failed to extract backup archive: %w", err)
	}

	// Perform atomic swap: backup current data, then restore
	backupDataDir := filepath.Join(os.TempDir(), fmt.Sprintf("stackarr-old-data-%d", time.Now().UnixNano()))

	progressChan <- streaming.ProgressEvent{
		Type:      "atomic_swap",
		Message:   "Performing atomic data swap...",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Rename current data directory (with cross-device fallback)
	if err := os.Rename(dataDir, backupDataDir); err != nil {
		// Check if it's a cross-device error, if so use copy+delete instead
		if linkErr, ok := err.(*os.LinkError); ok && linkErr.Err.Error() == "invalid cross-device link" {
			log.Warn().Msg("Cross-device link detected, using copy+delete fallback")
			// Copy the directory recursively
			if copyErr := copyDir(dataDir, backupDataDir); copyErr != nil {
				return fmt.Errorf("failed to copy current data directory: %w", copyErr)
			}
			// Remove the original directory
			if rmErr := os.RemoveAll(dataDir); rmErr != nil {
				return fmt.Errorf("failed to remove original data directory after copy: %w", rmErr)
			}
		} else {
			return fmt.Errorf("failed to backup current data directory: %w", err)
		}
	}

	// Rename restored data to data directory (with cross-device fallback)
	if err := os.Rename(tempRestoreDir, dataDir); err != nil {
		// Check if it's a cross-device error, if so use copy+delete instead
		if linkErr, ok := err.(*os.LinkError); ok && linkErr.Err.Error() == "invalid cross-device link" {
			log.Warn().Msg("Cross-device link detected for restored data, using copy+delete fallback")
			// Copy the restored data
			if copyErr := copyDir(tempRestoreDir, dataDir); copyErr != nil {
				// Try to restore the old data if swap failed
				if restoreErr := restoreBackupData(backupDataDir, dataDir); restoreErr != nil {
					log.Error().Err(restoreErr).Msg("CRITICAL: Failed to restore original data after swap failure")
				}
				return fmt.Errorf("failed to copy restored data into place: %w", copyErr)
			}
			// Remove the temporary restore directory
			if rmErr := os.RemoveAll(tempRestoreDir); rmErr != nil {
				log.Warn().Err(rmErr).Msg("Failed to remove temporary restore directory")
			}
		} else {
			// Try to restore the old data if swap failed
			if restoreErr := restoreBackupData(backupDataDir, dataDir); restoreErr != nil {
				log.Error().Err(restoreErr).Msg("CRITICAL: Failed to restore original data after swap failure")
			}
			return fmt.Errorf("failed to swap restored data into place: %w", err)
		}
	}

	// Clean up the old data directory
	os.RemoveAll(backupDataDir)

	log.Info().Msg("Restore completed successfully")

	progressChan <- streaming.ProgressEvent{
		Type:      "complete",
		Message:   "Restore completed successfully",
		Timestamp: time.Now().Format(time.RFC3339),
		Data: map[string]string{
			"redirect":      "/dashboard",
			"safety_backup": preRestoreBackupPath,
		},
	}

	return nil
}

// ValidateBackup validates a backup archive
func ValidateBackup(archivePath string) (*Manifest, error) {
	// Check if file exists
	if _, err := os.Stat(archivePath); err != nil {
		return nil, fmt.Errorf("backup file does not exist: %w", err)
	}

	// Open ZIP file
	zipReader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open backup archive: %w", err)
	}
	defer zipReader.Close()

	var manifestFile *zip.File
	var manifest *Manifest

	// Find and read manifest
	for _, f := range zipReader.File {
		if f.Name == "manifest.json" {
			manifestFile = f
			break
		}
	}

	if manifestFile == nil {
		return nil, fmt.Errorf("manifest.json not found in backup archive")
	}

	// Read manifest content
	rc, err := manifestFile.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open manifest in archive: %w", err)
	}
	defer rc.Close()

	manifestData, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	manifest = &Manifest{}
	if err := json.Unmarshal(manifestData, manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Check for required database file
	if len(manifest.DatabaseFiles) == 0 {
		return nil, fmt.Errorf("backup contains no database files")
	}

	// Verify version compatibility
	if manifest.Version != "1.0" {
		log.Warn().Str("version", manifest.Version).Msg("Backup manifest has different version")
		// Don't fail - allow restoring backups with compatible versions
	}

	return manifest, nil
}

// Helper functions

// addFileToZip adds a file to a ZIP archive
func addFileToZip(zipWriter *zip.Writer, filePath string, zipPath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	// Get file info
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	// Create ZIP entry with file info
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("failed to create ZIP header for %s: %w", filePath, err)
	}
	header.Name = zipPath
	header.Method = zip.Deflate

	// Write file to ZIP
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("failed to create ZIP entry for %s: %w", filePath, err)
	}

	if _, err := io.Copy(writer, file); err != nil {
		return fmt.Errorf("failed to write file to ZIP: %w", err)
	}

	return nil
}

// checkpointWAL checkpoints the WAL file to flush pending writes
func checkpointWAL(db *sql.DB) error {
	_, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	return err
}

// restoreBackupData restores backup data to the data directory, handling cross-device links
func restoreBackupData(backupDir, dataDir string) error {
	// First try rename
	if err := os.Rename(backupDir, dataDir); err != nil {
		// If cross-device error, use copy+delete
		if linkErr, ok := err.(*os.LinkError); ok && linkErr.Err.Error() == "invalid cross-device link" {
			log.Warn().Msg("Cross-device link detected during backup restore, using copy+delete fallback")
			if copyErr := copyDir(backupDir, dataDir); copyErr != nil {
				return copyErr
			}
			return os.RemoveAll(backupDir)
		}
		return err
	}
	return nil
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	// Get the source directory info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source directory: %w", err)
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Read source directory entries
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectory
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Get source file info
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()

	// Copy file contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Preserve file permissions
	if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	return nil
}

// extractBackupArchive extracts a backup archive to a directory
func extractBackupArchive(archivePath string, destDir string, progressChan chan<- streaming.ProgressEvent) error {
	zipReader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open backup archive: %w", err)
	}
	defer zipReader.Close()

	// Count total files
	totalFiles := len(zipReader.File)

	progressChan <- streaming.ProgressEvent{
		Type:      "extract_file",
		Message:   fmt.Sprintf("Extracting %d files from backup...", totalFiles),
		Timestamp: time.Now().Format(time.RFC3339),
		Data: map[string]interface{}{
			"total": totalFiles,
		},
	}

	// Extract each file
	for i, f := range zipReader.File {
		// Skip directories
		if f.FileInfo().IsDir() {
			continue
		}

		// Get the file path
		destPath := filepath.Join(destDir, f.Name)

		// Create directories if needed
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", destPath, err)
		}

		// Open the file in the archive
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in archive: %w", err)
		}

		// Create the destination file
		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file %s: %w", destPath, err)
		}

		// Copy file contents
		if _, err := io.Copy(outFile, rc); err != nil {
			outFile.Close()
			rc.Close()
			return fmt.Errorf("failed to extract file %s: %w", destPath, err)
		}

		outFile.Close()
		rc.Close()

		progressChan <- streaming.ProgressEvent{
			Type:      "extract_file",
			Message:   fmt.Sprintf("Extracting %s", f.Name),
			Timestamp: time.Now().Format(time.RFC3339),
			Data: map[string]interface{}{
				"current":  i + 1,
				"total":    totalFiles,
				"filename": f.Name,
			},
		}
	}

	return nil
}
