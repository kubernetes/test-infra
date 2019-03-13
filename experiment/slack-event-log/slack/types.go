/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package slack

import "encoding/json"

// User represents a slack User object.
type User struct {
	ID             string `json:"id"`
	TeamID         string `json:"team_id"`
	Name           string `json:"name"`
	Deleted        bool   `json:"deleted"`
	Color          string `json:"color"`
	TimeZone       string `json:"tz"`
	TimeZoneLabel  string `json:"tz_label"`
	TimeZoneOffset int    `json:"tz_offset"`
	Profile        struct {
		AvatarHash       string `json:"avatar_hash"`
		StatusText       string `json:"status_text"`
		StatusEmoji      string `json:"status_emoji"`
		StatusExpiration int    `json:"status_expiration"`
		RealName         string `json:"real_name"`
		DisplayName      string `json:"display_name"`
		Email            string `json:"email,omitempty"`
		ImageOriginal    string `json:"image_original"`
		Image24          string `json:"image_24"`
		Image32          string `json:"image_32"`
		Image48          string `json:"image_48"`
		Image72          string `json:"image_72"`
		Image192         string `json:"image_192"`
		Image512         string `json:"image_512"`
		Team             string `json:"team"`
	} `json:"profile"`
	IsAdmin           bool   `json:"is_admin"`
	IsOwner           bool   `json:"is_owner"`
	IsPrimaryOwner    bool   `json:"is_primary_owner"`
	IsRestricted      bool   `json:"is_restricted"`
	IsUltraRestricted bool   `json:"is_ultra_restricted"`
	IsBot             bool   `json:"is_bot"`
	IsStranger        bool   `json:"is_stranger"`
	IsAppUser         bool   `json:"is_app_user"`
	Has2FA            bool   `json:"has_2fa"`
	Locale            string `json:"locale"`
}

// Subteam represents a slack Subteam object.
type Subteam struct {
	ID          string `json:"id"`
	IsUsergroup bool   `json:"is_usergroup"`
	Name        string `json:"name"`
	Handle      string `json:"handle"`
	UserCount   int    `json:"user_count"`
	UpdatedBy   string `json:"updated_by"`
	CreatedBy   string `json:"created_by"`
	DeletedBy   string `json:"deleted_by"`
	CreateTime  int    `json:"date_create"`
	UpdateTime  int    `json:"date_update"`
	DeleteTime  int    `json:"date_delete"`
}

// DialogWrapper is the root object in a request to dialog.open
type DialogWrapper struct {
	TriggerID string `json:"trigger_id"`
	Dialog    Dialog `json:"dialog"`
}

// Dialog represents a dialog opened by dialog.open.
type Dialog struct {
	CallbackID     string        `json:"callback_id"`
	Title          string        `json:"title"`
	SubmitLabel    string        `json:"submit_label,omitempty"`
	NotifyOnCancel bool          `json:"notify_on_cancel,omitempty"`
	State          string        `json:"state,omitempty"`
	Elements       []interface{} `json:"elements"`
}

// TextArea represents a TextArea
type TextArea struct {
	Type        textAreaType `json:"type"`
	Label       string       `json:"label"`
	Name        string       `json:"name"`
	Placeholder string       `json:"placeholder,omitempty"`
	MaxLength   int          `json:"max_length,omitempty"`
	MinLength   int          `json:"min_length,omitempty"`
	Optional    bool         `json:"optional,omitempty"`
	Hint        string       `json:"hint,omitempty"`
	Subtype     string       `json:"subtype,omitempty"`
	Value       string       `json:"value,omitempty"`
}
type textAreaType string

func (textAreaType) MarshalJSON() ([]byte, error) {
	return json.Marshal("textarea")
}

// SelectElement represents a SelectElement
type SelectElement struct {
	Label           string         `json:"label"`
	Name            string         `json:"name"`
	Type            selectType     `json:"type"`
	DataSource      string         `json:"data_source,omitempty"`
	MinQueryLength  int            `json:"min_query_length,omitempty"`
	Placeholder     string         `json:"placeholder,omitempty"`
	Optional        bool           `json:"optional,omitempty"`
	Value           string         `json:"value,omitempty"`
	SelectedOptions []SelectOption `json:"selected_options,omitempty"`
	Options         []SelectOption `json:"options,omitempty"`
}
type selectType string

func (selectType) MarshalJSON() ([]byte, error) {
	return json.Marshal("select")
}

// SelectionOption represents a single option in a SelectElement
type SelectOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// SelectGroup represents a group of SelectOptions in a SelectElement.
type SelectGroup struct {
	Label   string         `json:"label"`
	Options []SelectOption `json:"options"`
}
