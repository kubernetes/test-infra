/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	admissionapi "k8s.io/api/admission/v1beta1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestOnlyUpdateStatus(t *testing.T) {
	cases := []struct {
		name     string
		sub      string
		old      prowjobv1.ProwJob
		new      prowjobv1.ProwJob
		expected *admissionapi.AdmissionResponse
	}{{
		name:     "allow status updates",
		sub:      "status",
		expected: &allow,
	}, {
		name: "reject different specs",
		old: prowjobv1.ProwJob{
			Spec: prowjobv1.ProwJobSpec{
				MaxConcurrency: 1,
			},
		},
		new: prowjobv1.ProwJob{
			Spec: prowjobv1.ProwJobSpec{
				MaxConcurrency: 2,
			},
		},
		expected: &reject,
	}, {

		name: "allow changes with same spec",
		old: prowjobv1.ProwJob{
			Status: prowjobv1.ProwJobStatus{
				State: prowjobv1.PendingState,
			},
			Spec: prowjobv1.ProwJobSpec{
				MaxConcurrency: 2,
			},
		},
		new: prowjobv1.ProwJob{
			Status: prowjobv1.ProwJobStatus{
				State: prowjobv1.SuccessState,
			},
			Spec: prowjobv1.ProwJobSpec{
				MaxConcurrency: 2,
			},
		},
		expected: &allow,
	}, {
		name: "allow changes with no changes",
		old: prowjobv1.ProwJob{
			Spec: prowjobv1.ProwJobSpec{
				MaxConcurrency: 2,
			},
		},
		new: prowjobv1.ProwJob{
			Spec: prowjobv1.ProwJobSpec{
				MaxConcurrency: 2,
			},
		},
		expected: &allow,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req admissionapi.AdmissionRequest
			var err error
			req.SubResource = tc.sub
			req.Object.Raw, err = json.Marshal(tc.new)
			if err != nil {
				t.Fatalf("encode new: %v", err)
			}
			req.OldObject.Raw, err = json.Marshal(tc.old)
			if err != nil {
				t.Fatalf("encode old: %v", err)
			}
			actual, err := onlyUpdateStatus(req)
			switch {
			case tc.expected == nil:
				if err == nil {
					t.Errorf("failed to receive an exception")
				}
			case err != nil:
				t.Errorf("unexpected error: %v", err)
			case !reflect.DeepEqual(actual, tc.expected):
				t.Errorf("actual %#v != expected %#v", actual, tc.expected)
			}
		})
	}
}

func TestWriteResponse(t *testing.T) {
	cases := []struct {
		name     string
		req      admissionapi.AdmissionRequest
		resp     *admissionapi.AdmissionResponse
		respErr  error
		writeErr bool
		expected *admissionapi.AdmissionReview
	}{
		{
			name: "include request UID in output",
			req: admissionapi.AdmissionRequest{
				UID: "123",
			},
			resp: &admissionapi.AdmissionResponse{},
			expected: &admissionapi.AdmissionReview{
				Response: &admissionapi.AdmissionResponse{
					UID: "123",
				},
			},
		},
		{
			name: "include response in output",
			resp: &admissionapi.AdmissionResponse{
				Allowed: true,
				Result: &meta.Status{
					Reason:  meta.StatusReasonForbidden,
					Message: "yo",
				},
			},
			expected: &admissionapi.AdmissionReview{
				Response: &admissionapi.AdmissionResponse{
					Allowed: true,
					Result: &meta.Status{
						Reason:  meta.StatusReasonForbidden,
						Message: "yo",
					},
				},
			},
		},
		{
			name:    "create response when decision fails",
			respErr: errors.New("hey there"),
			expected: &admissionapi.AdmissionReview{
				Response: &admissionapi.AdmissionResponse{
					Result: &meta.Status{
						Message: errors.New("hey there").Error(),
					},
				},
			},
		},
		{
			name: "error when writing fails",
			resp: &admissionapi.AdmissionResponse{
				Allowed: true,
			},
			writeErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fw := fakeWriter{
				w:   bytes.Buffer{},
				err: tc.writeErr,
			}
			decide := func(ar admissionapi.AdmissionRequest) (*admissionapi.AdmissionResponse, error) {
				if !reflect.DeepEqual(ar, tc.req) {
					return nil, fmt.Errorf("request %#v != expected %#v", ar, tc.req)
				}
				if tc.respErr != nil {
					return nil, tc.respErr
				}
				return tc.resp, nil
			}
			err := writeResponse(tc.req, &fw, decide)
			switch {
			case err != nil:
				if tc.expected != nil {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.expected == nil:
				t.Error("failed to receive error")
			default:
				expected, err := json.Marshal(*tc.expected)
				if err != nil {
					t.Fatalf("marhsal expected: %v", err)
				}
				if buf := fw.w.Bytes(); !reflect.DeepEqual(expected, buf) {
					t.Errorf("actual %s != expected %s", buf, expected)
				}
			}
		})
	}
}

type fakeWriter struct {
	w   bytes.Buffer
	err bool
}

func (w *fakeWriter) Write(p []byte) (int, error) {
	if w.err {
		return 0, errors.New("injected write error")
	}
	return w.w.Write(p)
}

type fakeReader struct {
	bytes bytes.Buffer
	err   bool
}

func (r *fakeReader) Read(p []byte) (int, error) {
	if r.err {
		return 0, errors.New("injected read error")
	}
	return r.bytes.Read(p)
}

func TestReadRequest(t *testing.T) {
	cases := []struct {
		name     string
		ct       string
		data     *admissionapi.AdmissionReview
		bytes    []byte
		readErr  bool
		expected *admissionapi.AdmissionRequest
	}{
		{
			name: "error on bad content type",
			ct:   "text/html",
		},
		{
			name: "error on empty body",
		},
		{
			name:    "error on read error",
			readErr: true,
		},
		{
			name:  "error on decode error",
			bytes: []byte("this is not valid json"),
		},
		{
			name: "return decoded review",
			data: &admissionapi.AdmissionReview{
				Request: &admissionapi.AdmissionRequest{
					SubResource: "status",
				},
			},
			expected: &admissionapi.AdmissionRequest{
				SubResource: "status",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fr := &fakeReader{
				bytes: *bytes.NewBuffer(tc.bytes),
				err:   tc.readErr,
			}
			if tc.data != nil {
				if err := codecs.LegacyCodec(admissionapi.SchemeGroupVersion).Encode(tc.data, &fr.bytes); err != nil {
					t.Fatalf("failed encoding: %v", err)
				}
			}
			if len(tc.ct) == 0 {
				tc.ct = contentTypeJSON
			}
			actual, err := readRequest(fr, tc.ct)
			switch {
			case actual == nil:
				if tc.expected != nil {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.expected == nil:
				t.Error("failed to receive error")
			case !reflect.DeepEqual(actual, tc.expected):
				t.Errorf("return %#v != expected %#v", actual, tc.expected)
			}
		})
	}
}
