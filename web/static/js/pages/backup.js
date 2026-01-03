/**
 * Backup & Restore Module
 * Handles user interactions for backup creation and restore operations
 */

function openBackupModal() {
    const modal = document.getElementById('backupModal');
    modal.classList.add('active');
    modal.querySelector('.modal-backdrop').addEventListener('click', closeBackupModal);
    document.addEventListener('keydown', handleBackupModalEscape);
}

function closeBackupModal() {
    const modal = document.getElementById('backupModal');
    modal.classList.remove('active');
    document.removeEventListener('keydown', handleBackupModalEscape);

    // Reset form state
    document.getElementById('backupFileInput').value = '';
    document.getElementById('restoreBackupBtn').disabled = true;
    document.getElementById('backupProgress').style.display = 'none';

    // Reset file upload label
    const label = document.querySelector('.file-upload-label');
    label.classList.remove('file-selected');
    label.innerHTML = '<i data-feather="upload"></i> Choose backup file';
    feather.replace();
}

function handleBackupModalEscape(event) {
    if (event.key === 'Escape') {
        closeBackupModal();
    }
}

async function handleCreateBackup() {
    const btn = document.getElementById('createBackupBtn');
    const originalText = btn.innerHTML;

    try {
        // Show loading overlay
        document.getElementById('loadingOverlay').classList.add('active');
        btn.disabled = true;

        // Show progress
        document.getElementById('backupProgress').style.display = 'block';
        updateBackupStatus('Initiating backup...');

        // Initiate backup creation
        const response = await fetch('/backup/create', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            }
        });

        if (!response.ok) {
            throw new Error('Failed to initiate backup');
        }

        const data = await response.json();
        const operationID = data.operation_id;

        // Track progress with custom handling for backup
        const progress = new ContainerOperationProgress(operationID);
        let intentionallyClosed = false;

        // Override complete handler to download file
        const originalComplete = progress.handleComplete.bind(progress);
        progress.handleComplete = function(data) {
            console.log('Backup complete', data);
            intentionallyClosed = true;
            // Close the EventSource connection first
            if (this.eventSource) {
                this.eventSource.close();
            }

            if (data.download_url) {
                // Download the file
                updateBackupStatus('✓ Backup complete! Downloading...');
                setTimeout(() => {
                    window.location.href = data.download_url;
                    setTimeout(() => {
                        closeBackupModal();
                        document.getElementById('loadingOverlay').classList.remove('active');
                    }, 500);
                }, 300);
            } else {
                originalComplete(data);
            }
        };

        // Override error handler to suppress error after intentional close
        const originalError = progress.handleError.bind(progress);
        progress.handleError = function(message) {
            // Don't show error if we intentionally closed the connection
            if (intentionallyClosed) {
                return;
            }
            document.getElementById('loadingOverlay').classList.remove('active');
            btn.disabled = false;
            btn.innerHTML = originalText;
            originalError(message);
        };

        progress.start();

    } catch (error) {
        console.error('Backup error:', error);
        document.getElementById('loadingOverlay').classList.remove('active');
        btn.disabled = false;
        btn.innerHTML = originalText;
        alert('Error: ' + error.message);
    }
}

function handleFileSelect(event) {
    const file = event.target.files[0];
    const btn = document.getElementById('restoreBackupBtn');
    const label = document.querySelector('.file-upload-label');

    if (file && file.name.endsWith('.zip')) {
        btn.disabled = false;
        label.classList.add('file-selected');
        label.innerHTML = '<i data-feather="check-circle"></i> ' + file.name;
        // Re-render feather icons
        feather.replace();
    } else {
        btn.disabled = true;
        label.classList.remove('file-selected');
        label.innerHTML = '<i data-feather="upload"></i> Choose backup file';
        // Re-render feather icons
        feather.replace();
    }
}

async function handleRestoreBackup() {
    const fileInput = document.getElementById('backupFileInput');
    const file = fileInput.files[0];

    if (!file) {
        alert('Please select a backup file');
        return;
    }

    // Show confirmation dialog
    const confirmed = confirm(
        'WARNING: This will stop all containers and restore your configuration from the backup. ' +
        'A safety backup of your current data will be created automatically. ' +
        'Continue?'
    );

    if (!confirmed) {
        return;
    }

    const btn = document.getElementById('restoreBackupBtn');
    const originalText = btn.innerHTML;

    try {
        document.getElementById('loadingOverlay').classList.add('active');
        btn.disabled = true;

        // Show progress
        document.getElementById('backupProgress').style.display = 'block';
        updateBackupStatus('Uploading backup file...');

        // Upload the backup file
        const formData = new FormData();
        formData.append('backup_file', file);

        const uploadResponse = await fetch('/backup/upload', {
            method: 'POST',
            body: formData
        });

        if (!uploadResponse.ok) {
            const error = await uploadResponse.text();
            throw new Error(error || 'Failed to upload backup file');
        }

        const uploadData = await uploadResponse.json();
        const backupPath = uploadData.path;

        updateBackupStatus('Starting restore operation...');

        // Initiate restore
        const restoreResponse = await fetch('/backup/restore', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                backup_path: backupPath
            })
        });

        if (!restoreResponse.ok) {
            throw new Error('Failed to initiate restore');
        }

        const restoreData = await restoreResponse.json();
        const operationID = restoreData.operation_id;

        // Track progress
        const progress = new ContainerOperationProgress(operationID);
        let restoreIntentionallyClosed = false;

        // Override error handler to show safety backup info
        const originalError = progress.handleError.bind(progress);
        progress.handleError = function(message) {
            // Don't show error if we intentionally closed the connection
            if (restoreIntentionallyClosed) {
                return;
            }
            document.getElementById('loadingOverlay').classList.remove('active');
            btn.disabled = false;
            btn.innerHTML = originalText;
            alert('Restore failed: ' + message + '\n\nA safety backup of your original data has been created. Please check the logs for details.');
        };

        // Override complete handler to handle page reload on success
        const originalComplete = progress.handleComplete.bind(progress);
        progress.handleComplete = function(data) {
            restoreIntentionallyClosed = true;
            if (this.eventSource) {
                this.eventSource.close();
            }
            // Use the original handler which redirects to dashboard
            originalComplete(data);
        };

        progress.start();

    } catch (error) {
        console.error('Restore error:', error);
        document.getElementById('loadingOverlay').classList.remove('active');
        btn.disabled = false;
        btn.innerHTML = originalText;
        alert('Error: ' + error.message);
    }
}

function updateBackupStatus(message) {
    const statusEl = document.getElementById('backupStatus');
    if (statusEl) {
        statusEl.textContent = message;
    }
}

// Prevent modal from closing when clicking inside content
document.addEventListener('DOMContentLoaded', function() {
    const modalContent = document.querySelector('.backup-modal .modal-content');
    if (modalContent) {
        modalContent.addEventListener('click', function(e) {
            e.stopPropagation();
        });
    }

    const backdrop = document.querySelector('.backup-modal .modal-backdrop');
    if (backdrop) {
        backdrop.addEventListener('click', closeBackupModal);
    }
});
