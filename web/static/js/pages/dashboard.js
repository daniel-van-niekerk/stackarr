/**
 * Dashboard page specific JavaScript
 * Handles container action forms and loading states
 */

document.addEventListener('DOMContentLoaded', function() {
    const loadingOverlay = document.getElementById('loadingOverlay');
    const loadingText = document.getElementById('loadingText');

    // Get all forms that trigger container actions
    const forms = document.querySelectorAll('form[action*="/containers/"]');

    forms.forEach(form => {
        form.addEventListener('submit', async function(e) {
            const action = form.getAttribute('action');

            // Set appropriate loading message based on action
            if (action.includes('/containers/update')) {
                e.preventDefault();

                // Reset UI
                loadingText.style.display = 'block';
                loadingText.textContent = 'Updating container, please wait...';
                document.getElementById('progressStatus').style.display = 'none';
                document.getElementById('progressBarContainer').style.display = 'none';
                document.getElementById('layerProgressContainer').innerHTML = '';
                loadingOverlay.classList.add('active');

                // Submit via AJAX
                try {
                    const response = await fetch(action, {
                        method: 'POST'
                    });

                    const result = await response.json();

                    if (result.operation_id) {
                        const progress = new ContainerOperationProgress(result.operation_id);
                        progress.start();
                    } else {
                        throw new Error('No operation ID returned');
                    }
                } catch (error) {
                    console.error('Error:', error);
                    alert('Failed to start update: ' + error.message);
                    loadingOverlay.classList.remove('active');
                }
            } else if (action.includes('/containers/start')) {
                loadingText.textContent = 'Starting container...';
                loadingOverlay.classList.add('active');
            } else if (action.includes('/containers/stop')) {
                loadingText.textContent = 'Stopping container...';
                loadingOverlay.classList.add('active');
            } else if (action.includes('/containers/restart')) {
                loadingText.textContent = 'Restarting container...';
                loadingOverlay.classList.add('active');
            } else if (action.includes('/containers/delete')) {
                // Don't show spinner for delete, let the confirm dialog handle it
                return;
            }
        });
    });
});
