/**
 * Confirmation Dialog - Handles confirmation modals for container actions
 */

let confirmDialog = {
    modal: null,
    form: null,
    action: null,
    containerName: null
};

/**
 * Initialize the confirm dialog
 */
function initConfirmDialog() {
    confirmDialog.modal = document.getElementById('confirmModal');
}

/**
 * Show confirmation dialog for an action
 * @param {string} action - The action type (delete, stop, restart, update)
 * @param {string} containerName - The container name for personalized message
 * @param {HTMLFormElement} form - The form to submit after confirmation
 */
function showConfirmDialog(action, containerName, form) {
    if (!confirmDialog.modal) {
        initConfirmDialog();
    }

    if (!form) {
        console.error('Form element not found for action:', action);
        alert('Error: Could not find form element');
        return;
    }

    confirmDialog.action = action;
    confirmDialog.containerName = containerName;
    confirmDialog.form = form;

    const titleEl = document.getElementById('confirmTitle');
    const messageEl = document.getElementById('confirmMessage');
    const buttonEl = document.getElementById('confirmButton');

    // Get action-specific text and styling
    const actionConfig = getActionConfig(action);

    titleEl.textContent = actionConfig.title;
    messageEl.textContent = actionConfig.message.replace('{name}', containerName);

    // Reset and apply button styling
    buttonEl.className = 'btn-confirm ' + actionConfig.buttonStyle;
    buttonEl.textContent = actionConfig.buttonText;

    confirmDialog.modal.classList.add('active');
}

/**
 * Get configuration for a specific action
 * @param {string} action - The action type
 * @returns {Object} Configuration object
 */
function getActionConfig(action) {
    const configs = {
        delete: {
            title: 'Delete Container',
            message: 'Are you sure you want to delete {name}? This cannot be undone.',
            buttonText: 'Delete',
            buttonStyle: 'danger'
        },
        stop: {
            title: 'Stop Container',
            message: 'Are you sure you want to stop {name}?',
            buttonText: 'Stop',
            buttonStyle: 'warning'
        },
        restart: {
            title: 'Restart Container',
            message: 'Are you sure you want to restart {name}?',
            buttonText: 'Restart',
            buttonStyle: 'warning'
        },
        start: {
            title: 'Start Container',
            message: 'Are you sure you want to start {name}?',
            buttonText: 'Start',
            buttonStyle: 'info'
        },
        update: {
            title: 'Update Container',
            message: 'Are you sure you want to update {name}?',
            buttonText: 'Update',
            buttonStyle: 'info'
        }
    };

    return configs[action] || configs.update;
}

/**
 * Close the confirmation dialog
 */
function closeConfirmDialog() {
    if (confirmDialog.modal) {
        confirmDialog.modal.classList.remove('active');
        clearConfirmState();
    }
}

/**
 * Confirm the action and submit the form
 */
function confirmAction() {
    const formToSubmit = confirmDialog.form;

    if (formToSubmit) {
        // Trigger submit event to allow dashboard.js handlers to process
        const submitEvent = new Event('submit', { bubbles: true, cancelable: true });
        formToSubmit.dispatchEvent(submitEvent);

        // Close the dialog after triggering the event
        closeConfirmDialog();

        // Only submit the form if preventDefault wasn't called
        // For actions like update that use fetch, preventDefault is called
        // For other actions, preventDefault is not called and form should submit normally
        setTimeout(() => {
            if (!submitEvent.defaultPrevented) {
                formToSubmit.submit();
            }
        }, 0);
    }
}

/**
 * Clear confirmation dialog state
 */
function clearConfirmState() {
    confirmDialog.action = null;
    confirmDialog.containerName = null;
    confirmDialog.form = null;
}

/**
 * Close modal on Escape key
 */
document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape' && confirmDialog.modal && confirmDialog.modal.classList.contains('active')) {
        closeConfirmDialog();
    }
});

/**
 * Initialize dialog when DOM is ready
 */
document.addEventListener('DOMContentLoaded', initConfirmDialog);
