package v1beta1

type Error struct {
	AppCode string      `json:"app_code,omitempty"`
	Message string      `json:"message,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

func (e Error) Error() string {
	return e.Message
}
