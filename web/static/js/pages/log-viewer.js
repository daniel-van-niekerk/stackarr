/**
 * Log Viewer - Handles container log viewing modal
 */

let currentContainerID = null;
let currentContainerName = null;
let currentLogs = '';

/**
 * Opens the log modal and fetches logs for a container
 */
function openLogModal(containerID, containerName) {
    currentContainerID = containerID;
    currentContainerName = containerName;

    const modal = document.getElementById('logModal');
    const nameEl = document.getElementById('logContainerName');

    nameEl.textContent = containerName;
    modal.classList.add('active');

    // Fetch logs
    fetchLogs();
}

/**
 * Closes the log modal
 */
function closeLogModal() {
    const modal = document.getElementById('logModal');
    modal.classList.remove('active');

    // Clear state
    currentContainerID = null;
    currentContainerName = null;
    currentLogs = '';
}

/**
 * Fetches logs from the server
 */
async function fetchLogs() {
    const loadingEl = document.getElementById('logLoadingSpinner');
    const contentEl = document.getElementById('logContent');
    const errorEl = document.getElementById('logError');

    // Show loading state
    loadingEl.style.display = 'flex';
    contentEl.style.display = 'none';
    errorEl.style.display = 'none';

    try {
        const response = await fetch(`/containers/logs?id=${encodeURIComponent(currentContainerID)}&tail=500`);

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const data = await response.json();
        currentLogs = data.logs;

        // Display logs
        loadingEl.style.display = 'none';

        if (!currentLogs || currentLogs.trim() === '') {
            contentEl.textContent = 'No logs available for this container.';
            contentEl.style.display = 'block';
        } else {
            contentEl.textContent = currentLogs;
            contentEl.style.display = 'block';

            // Auto-scroll to bottom
            setTimeout(() => {
                contentEl.scrollTop = contentEl.scrollHeight;
            }, 100);
        }
    } catch (error) {
        console.error('Error fetching logs:', error);
        loadingEl.style.display = 'none';
        errorEl.textContent = `Failed to fetch logs: ${error.message}`;
        errorEl.style.display = 'block';
    }
}

/**
 * Refreshes the current logs
 */
function refreshLogs() {
    if (currentContainerID) {
        fetchLogs();
    }
}

/**
 * Downloads the current logs as a text file
 */
function downloadLogs() {
    if (!currentLogs) {
        alert('No logs to download');
        return;
    }

    const blob = new Blob([currentLogs], { type: 'text/plain' });
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');

    const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
    a.href = url;
    a.download = `${currentContainerName}-logs-${timestamp}.txt`;

    document.body.appendChild(a);
    a.click();

    document.body.removeChild(a);
    window.URL.revokeObjectURL(url);
}

// Close modal on Escape key
document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') {
        closeLogModal();
    }
});
