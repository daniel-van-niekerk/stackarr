/**
 * ContainerOperationProgress - Handles SSE progress updates for container operations
 * Shared between dashboard and container form pages
 */
class ContainerOperationProgress {
    constructor(operationID) {
        this.operationID = operationID;
        this.eventSource = null;
        this.layerProgress = new Map();
    }

    start() {
        const url = `/containers/progress/${this.operationID}`;
        this.eventSource = new EventSource(url);

        this.eventSource.onmessage = (event) => {
            try {
                const progress = JSON.parse(event.data);
                this.handleProgressEvent(progress);
            } catch (e) {
                console.error('Failed to parse progress event:', e);
            }
        };

        this.eventSource.onerror = (error) => {
            console.error('SSE Error:', error);
            this.eventSource.close();
            this.showError('Connection lost. Please refresh the page.');
        };
    }

    handleProgressEvent(event) {
        console.log('Progress event:', event);

        switch(event.type) {
            case 'image_pull':
                this.updateStatus(event.message);
                break;

            case 'layer_download':
                this.updateLayerProgress(event.data);
                break;

            case 'container_stop':
            case 'container_remove':
            case 'container_create':
            case 'container_start':
            case 'database_update':
                this.updateStatus(event.message);
                break;

            case 'complete':
                this.handleComplete(event.data);
                break;

            case 'error':
                this.handleError(event.message);
                break;
        }
    }

    updateStatus(message) {
        const loadingTextEl = document.getElementById('loadingText');
        const progressStatusEl = document.getElementById('progressStatus');

        // Dashboard version: hide spinner, show status
        if (loadingTextEl) {
            loadingTextEl.style.display = 'none';
        }

        // Set status message
        if (progressStatusEl) {
            progressStatusEl.textContent = message;
            progressStatusEl.style.display = 'block';
        }
    }

    updateLayerProgress(layerData) {
        if (layerData) {
            this.layerProgress.set(layerData.layer_id, layerData);
            this.renderLayerProgress();
        }
    }

    renderLayerProgress() {
        const container = document.getElementById('layerProgressContainer');
        if (!container) return;

        container.innerHTML = '';

        if (this.layerProgress.size === 0) {
            return;
        }

        // Show progress bar container if it exists
        const progressBarContainer = document.getElementById('progressBarContainer');
        if (progressBarContainer) {
            progressBarContainer.style.display = 'block';
        }

        // Calculate overall progress
        let totalLayers = this.layerProgress.size;
        let completedLayers = 0;

        this.layerProgress.forEach((layer, id) => {
            if (layer.progress === 100 || layer.status === 'Pull complete') {
                completedLayers++;
            }

            const layerDiv = document.createElement('div');
            layerDiv.className = 'layer-progress-item';
            layerDiv.innerHTML = `
                <div class="layer-id">${id.substring(0, 12)}</div>
                <div class="layer-status">${layer.status}</div>
                <div class="layer-bar">
                    <div class="layer-bar-fill" style="width: ${layer.progress}%"></div>
                </div>
                <div class="layer-percentage">${layer.progress}%</div>
            `;
            container.appendChild(layerDiv);
        });

        // Update overall progress
        const overallProgress = totalLayers > 0 ? (completedLayers / totalLayers * 100) : 0;
        const overallProgressEl = document.getElementById('overallProgress');
        const overallPercentageEl = document.getElementById('overallPercentage');

        if (overallProgressEl) {
            overallProgressEl.style.width = `${overallProgress}%`;
        }
        if (overallPercentageEl) {
            overallPercentageEl.textContent = `${Math.round(overallProgress)}%`;
        }
    }

    handleComplete(data) {
        if (this.eventSource) {
            this.eventSource.close();
        }
        this.updateStatus('✓ Complete! Redirecting...');

        setTimeout(() => {
            window.location.href = data.redirect || '/dashboard';
        }, 1000);
    }

    handleError(message) {
        if (this.eventSource) {
            this.eventSource.close();
        }
        this.showError(message);
    }

    showError(message) {
        document.getElementById('loadingOverlay').classList.remove('active');
        alert('Error: ' + message);
    }
}
