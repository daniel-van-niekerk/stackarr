/**
 * Container form page specific JavaScript
 * Handles dynamic form fields and form submission with progress tracking
 */

// Remove item from dynamic list
function removeItem(btn) {
    btn.parentElement.remove();
}

// Add port mapping to the list
function addPort() {
    const list = document.getElementById('ports-list');
    const item = document.createElement('div');
    item.className = 'dynamic-item';
    item.innerHTML = `
        <input type="number" name="port_host[]" placeholder="Host Port" min="1" max="65535">
        <input type="number" name="port_container[]" placeholder="Container Port" min="1" max="65535">
        <select name="port_protocol[]">
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
        </select>
        <button type="button" class="btn-remove" onclick="removeItem(this)">×</button>
    `;
    list.appendChild(item);
}

// Add volume mapping to the list
function addVolume() {
    const list = document.getElementById('volumes-list');
    const item = document.createElement('div');
    item.className = 'dynamic-item';
    item.innerHTML = `
        <input type="text" name="volume_host[]" placeholder="Host Path">
        <input type="text" name="volume_container[]" placeholder="Container Path">
        <button type="button" class="btn-remove" onclick="removeItem(this)">×</button>
    `;
    list.appendChild(item);
}

// Add environment variable to the list
function addEnvVar() {
    const list = document.getElementById('env-list');
    const item = document.createElement('div');
    item.className = 'dynamic-item';
    item.innerHTML = `
        <input type="text" name="env_key[]" placeholder="Key">
        <input type="text" name="env_value[]" placeholder="Value">
        <button type="button" class="btn-remove" onclick="removeItem(this)">×</button>
    `;
    list.appendChild(item);
}

// Initialize form on page load
window.addEventListener('DOMContentLoaded', function() {
    // Add initial empty rows if none exist
    const portsList = document.getElementById('ports-list');
    const volumesList = document.getElementById('volumes-list');
    const envList = document.getElementById('env-list');

    if (portsList && portsList.children.length === 0) {
        addPort();
    }
    if (volumesList && volumesList.children.length === 0) {
        addVolume();
    }
    if (envList && envList.children.length === 0) {
        addEnvVar();
    }

    // Handle form submission with AJAX + SSE
    const form = document.getElementById('containerForm');
    if (form) {
        form.addEventListener('submit', async function(e) {
            e.preventDefault();

            // Show loading overlay with progress UI
            const overlay = document.getElementById('loadingOverlay');
            overlay.classList.add('active');

            // Reset progress UI
            const progressStatus = document.getElementById('progressStatus');
            if (progressStatus) {
                progressStatus.textContent = 'Initializing...';
            }

            const overallProgress = document.getElementById('overallProgress');
            if (overallProgress) {
                overallProgress.style.width = '0%';
            }

            const overallPercentage = document.getElementById('overallPercentage');
            if (overallPercentage) {
                overallPercentage.textContent = '0%';
            }

            const layerProgressContainer = document.getElementById('layerProgressContainer');
            if (layerProgressContainer) {
                layerProgressContainer.innerHTML = '';
            }

            // Submit form via AJAX
            const formData = new FormData(this);

            try {
                const response = await fetch('/containers/save', {
                    method: 'POST',
                    body: formData
                });

                const result = await response.json();

                if (!response.ok) {
                    // Handle error responses
                    throw new Error(result.error || 'Server error: ' + response.status);
                }

                if (result.operation_id) {
                    // Start SSE connection for async operations (container creation)
                    const progress = new ContainerOperationProgress(result.operation_id);
                    progress.start();
                } else if (result.status === 'success') {
                    // Handle synchronous updates (config changes only)
                    if (progressStatus) {
                        progressStatus.textContent = '✓ Complete! Redirecting...';
                    }
                    setTimeout(() => {
                        window.location.href = result.redirect || '/dashboard';
                    }, 500);
                } else {
                    throw new Error('Invalid response from server');
                }
            } catch (error) {
                console.error('Error:', error);
                alert('Error: ' + error.message);
                overlay.classList.remove('active');
            }
        });
    }
});
