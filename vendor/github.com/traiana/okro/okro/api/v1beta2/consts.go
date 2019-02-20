package v1beta2

const (
	// AppCodes
	ValidationError = "validation_failed"
	MergeError      = "merge_failed"
	MergeWarning    = "merge_warning"
	OkExisting      = "ok_existing"
	OkNew           = "ok_new"

	// Headers
	CommitValidationHeader = "X-Validate-Against"
	UpdatedByHeader        = "X-Updated-By"
)
