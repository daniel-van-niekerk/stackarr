package types

// ContainerState represents the state of a container
type ContainerState string

const (
	StateRunning ContainerState = "running"
	StateStopped ContainerState = "stopped"
	StateDeleted ContainerState = "deleted"
)

// ProgressEventType represents types of progress events during container operations
type ProgressEventType string

const (
	EventImagePull       ProgressEventType = "image_pull"
	EventLayerDownload   ProgressEventType = "layer_download"
	EventContainerStop   ProgressEventType = "container_stop"
	EventContainerRemove ProgressEventType = "container_remove"
	EventContainerCreate ProgressEventType = "container_create"
	EventContainerStart  ProgressEventType = "container_start"
	EventDatabaseUpdate  ProgressEventType = "database_update"
	EventComplete        ProgressEventType = "complete"
	EventError           ProgressEventType = "error"
)

// Container operation status messages
const (
	MsgDockerConnect          = "Connecting to Docker..."
	MsgImagePull              = "Pulling image..."
	MsgLayerDownload          = "Downloading layers..."
	MsgContainerCreate        = "Creating container..."
	MsgContainerStart         = "Starting container..."
	MsgContainerStop          = "Stopping container..."
	MsgContainerRemove        = "Removing container..."
	MsgDatabaseUpdate         = "Updating database..."
	MsgOperationComplete      = "Operation completed successfully"
	MsgOperationFailed        = "Operation failed"
)

// Default values
const (
	DefaultDarkMode = false
	DefaultUserRole = "user"
)

// Paths and configuration
const (
	VolumePermissions = 0777
	DefaultUserUID    = 1000
	DefaultUserGID    = 1000
)
