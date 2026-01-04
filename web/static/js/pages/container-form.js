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

// Detect browser timezone
function getBrowserTimezone() {
    try {
        return Intl.DateTimeFormat().resolvedOptions().timeZone;
    } catch (e) {
        return 'America/New_York'; // Fallback if detection fails
    }
}

// Check if TZ env var already exists
function hasTZVariable() {
    const envList = document.getElementById('env-list');
    for (let item of envList.children) {
        const keyInput = item.querySelector('input[name="env_key[]"]');
        if (keyInput && keyInput.value === 'TZ') {
            return true;
        }
    }
    return false;
}

// Set TZ to browser timezone if not already set or if it's the default
function setDefaultTimezone() {
    const timezone = getBrowserTimezone();
    const envList = document.getElementById('env-list');
    let tzFound = false;

    // Look for existing TZ variable
    for (let item of envList.children) {
        const keyInput = item.querySelector('input[name="env_key[]"]');
        const valueInput = item.querySelector('input[name="env_value[]"]');
        if (keyInput && keyInput.value === 'TZ') {
            tzFound = true;
            // Replace if it's the default value, otherwise keep user's choice
            if (valueInput.value === 'America/New_York') {
                valueInput.value = timezone;
            }
            return;
        }
    }

    // If no TZ found, add one
    if (!tzFound) {
        // Find the first empty row or create one
        for (let item of envList.children) {
            const keyInput = item.querySelector('input[name="env_key[]"]');
            const valueInput = item.querySelector('input[name="env_value[]"]');
            if (keyInput && !keyInput.value && !valueInput.value) {
                keyInput.value = 'TZ';
                valueInput.value = timezone;
                return;
            }
        }

        // If no empty row, add a new one
        addEnvVar();
        const lastItem = envList.lastElementChild;
        const keyInput = lastItem.querySelector('input[name="env_key[]"]');
        const valueInput = lastItem.querySelector('input[name="env_value[]"]');
        keyInput.value = 'TZ';
        valueInput.value = timezone;
    }
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

    // Set default timezone to browser timezone if not already set
    setDefaultTimezone();

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
