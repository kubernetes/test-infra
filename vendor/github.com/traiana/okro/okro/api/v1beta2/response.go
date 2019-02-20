package v1beta2

import "encoding/json"

type Error struct {
	AppCode string      `json:"app_code,omitempty"`
	Message string      `json:"message,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

func (e Error) Error() string {
	return e.Message
}

func (e Error) DetailedError() string {
	if e.Details == nil {
		return e.Message
	}

	var b []byte
	if s, ok := (e.Details).(string); ok {
		b = []byte(s)
	} else {
		var err error
		b, err = json.MarshalIndent(e.Details, "", "  ")
		if err != nil {
			return e.Message
		}
	}
	return string(b)
}

var clientFacing = []string{ValidationError, MergeError, MergeWarning}

func (e Error) IsClientFacing() bool {
	for _, code := range clientFacing {
		if e.AppCode == code {
			return true
		}
	}
	return false
}

type GenericResponse struct {
	AppCode string `json:"app_code,omitempty"`
	Message string `json:"message,omitempty"`
}
