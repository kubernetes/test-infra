package forcemerge

type ConstructError struct {
	message string
}

func (e *ConstructError) Error() string {
	return e.message
}

func NewConstructError(msg string) *ConstructError {
	return &ConstructError{msg}
}
