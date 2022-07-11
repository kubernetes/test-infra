/*
Copyright 2022 The Kubernetes Authors.

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

// Package criercommonlib contains shared lib used by reporters
package criercommonlib

import (
	"errors"
	"fmt"
)

type userError struct {
	err error
}

func (ue userError) Error() string {
	return fmt.Sprintf("this is a user error: %s", ue.err.Error())
}

func (userError) Is(err error) bool {
	_, ok := err.(userError)
	return ok
}

// UserError wraps an error and return a userError error.
func UserError(err error) error {
	return &userError{err: err}
}

// IsUserError checks whether the returned error is a user error.
func IsUserError(err error) bool {
	return errors.Is(err, userError{})
}
