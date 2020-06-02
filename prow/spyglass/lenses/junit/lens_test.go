/*
Copyright 2020 The Kubernetes Authors.

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

// Package junit provides a junit viewer for Spyglass
package junit

import (
	"bytes"
	"context"
	"io/ioutil"
	"testing"

	"github.com/GoogleCloudPlatform/testgrid/metadata/junit"
	"github.com/google/go-cmp/cmp"

	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

const (
	fakeResourceDir   = "resources-for-tests"
	fakeCanonicalLink = "linknotfound.io/404"
)

// FakeArtifact implements lenses.Artifact.
// This is pretty much copy/pasted from prow/spyglass/lenses/lenses_test.go, if
// another package needs to reuse, should think about refactor this into it's
// own package
type FakeArtifact struct {
	path      string
	content   []byte
	sizeLimit int64
}

func (fa *FakeArtifact) JobPath() string {
	return fa.path
}

func (fa *FakeArtifact) Size() (int64, error) {
	return int64(len(fa.content)), nil
}

func (fa *FakeArtifact) CanonicalLink() string {
	return fakeCanonicalLink
}

func (fa *FakeArtifact) ReadAt(b []byte, off int64) (int, error) {
	r := bytes.NewReader(fa.content)
	return r.ReadAt(b, off)
}

func (fa *FakeArtifact) ReadAll() ([]byte, error) {
	size, err := fa.Size()
	if err != nil {
		return nil, err
	}
	if size > fa.sizeLimit {
		return nil, lenses.ErrFileTooLarge
	}
	r := bytes.NewReader(fa.content)
	return ioutil.ReadAll(r)
}

func (fa *FakeArtifact) ReadTail(n int64) ([]byte, error) {
	return nil, nil
}

func (fa *FakeArtifact) UseContext(ctx context.Context) error {
	return nil
}

func (fa *FakeArtifact) ReadAtMost(n int64) ([]byte, error) {
	return nil, nil
}

func TestGetJvd(t *testing.T) {
	emptyFailureMsg := ""
	failureMsgs := []string{
		" failure message 0 ",
		" failure message 1 ",
	}

	tests := []struct {
		name       string
		rawResults [][]byte
		exp        JVD
	}{
		{
			"Failed",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed" type=""> failure message 0 </failure>
						</testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 1,
				Passed:   nil,
				Failed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   &failureMsgs[0],
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Skipped: nil,
				Flaky:   nil,
			},
		}, {
			"Passed",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0"></testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 1,
				Passed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   nil,
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Failed:  nil,
				Skipped: nil,
				Flaky:   nil,
			},
		}, {
			"Skipped",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<skipped/>
						</testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 1,
				Passed:   nil,
				Failed:   nil,
				Skipped: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   nil,
									Skipped:   &emptyFailureMsg,
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Flaky: nil,
			},
		}, {
			"Multiple tests in same file",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed" type=""> failure message 0 </failure>
						</testcase>
						<testcase classname="fake_class_1" name="fake_test_0"></testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 2,
				Passed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_1",
									Failure:   nil,
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Failed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   &failureMsgs[0],
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Skipped: nil,
				Flaky:   nil,
			},
		}, {
			"Multiple tests in different files",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed" type=""> failure message 0 </failure>
						</testcase>
					</testsuite>
				</testsuites>
				`),
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_1" name="fake_test_0"></testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 2,
				Passed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_1",
									Failure:   nil,
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Failed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   &failureMsgs[0],
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Skipped: nil,
				Flaky:   nil,
			},
		}, {
			"Fail multiple times in same file",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed" type=""> failure message 0 </failure>
						</testcase>
					</testsuite>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed" type=""> failure message 1 </failure>
						</testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 1,
				Passed:   nil,
				Failed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   &failureMsgs[0],
								},
							},
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   &failureMsgs[1],
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Skipped: nil,
				Flaky:   nil,
			},
		}, {
			"Passed multiple times in same file",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0"></testcase>
					</testsuite>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0"></testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 1,
				Passed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   nil,
								},
							},
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   nil,
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Failed:  nil,
				Skipped: nil,
				Flaky:   nil,
			},
		}, {
			// This is the case where `go test --count=N`, where N>1
			"Passed multiple times in same suite",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0"></testcase>
						<testcase classname="fake_class_0" name="fake_test_0"></testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 1,
				Passed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   nil,
								},
							},
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   nil,
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Failed:  nil,
				Skipped: nil,
				Flaky:   nil,
			},
		}, {
			"Failed then pass in same file (flaky)",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed" type=""> failure message 0 </failure>
						</testcase>
					</testsuite>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0"></testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 1,
				Passed:   nil,
				Failed:   nil,
				Skipped:  nil,
				Flaky: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   &failureMsgs[0],
								},
							},
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   nil,
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
			},
		}, {
			"Pass then fail in same file (flaky)",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0"></testcase>
					</testsuite>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed" type=""> failure message 0 </failure>
						</testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 1,
				Passed:   nil,
				Failed:   nil,
				Skipped:  nil,
				Flaky: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   nil,
								},
							},
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   &failureMsgs[0],
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
			},
		}, {
			"Fail multiple times then pass in same file (flaky)",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed" type=""> failure message 0 </failure>
						</testcase>
					</testsuite>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed" type=""> failure message 1 </failure>
						</testcase>
					</testsuite>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0"></testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 1,
				Passed:   nil,
				Failed:   nil,
				Skipped:  nil,
				Flaky: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   &failureMsgs[0],
								},
							},
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   &failureMsgs[1],
								},
							},
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   nil,
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
			},
		}, {
			"Same test in different file",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed" type=""> failure message 0 </failure>
						</testcase>
					</testsuite>
				</testsuites>
				`),
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0"></testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 2,
				Passed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   nil,
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Failed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:      "fake_test_0",
									ClassName: "fake_class_0",
									Failure:   &failureMsgs[0],
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Skipped: nil,
				Flaky:   nil,
			},
		},
		{
			"Test-cases with properties",
			[][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed once" type=""> failure message 0 </failure>
							<properties>
								<property name="a" value="b"/>
							</properties>
						</testcase>
						<testcase classname="fake_class_0" name="fake_test_1">
							<failure message="Failed twice" type=""> failure message 0 </failure>
							<properties>
								<property name="a" value="b"/>
							</properties>
						</testcase>
						<testcase classname="fake_class_0" name="fake_test_1">
							<failure message="Failed twice" type=""> failure message 1 </failure>
							<properties>
								<property name="c" value="d"/>
							</properties>
						</testcase>
						<testcase classname="fake_class_0" name="fake_test_2">
							<failure message="Flaked once" type=""> failure message 0 </failure>
							<properties>
								<property name="a" value="b"/>
							</properties>
						</testcase>
						<testcase classname="fake_class_0" name="fake_test_2">
							<properties>
								<property name="c" value="d"/>
							</properties>
						</testcase>
					</testsuite>
				</testsuites>
				`),
			},
			JVD{
				NumTests: 3,
				Failed: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:       "fake_test_0",
									ClassName:  "fake_class_0",
									Failure:    &failureMsgs[0],
									Properties: &junit.Properties{PropertyList: []junit.Property{{Name: "a", Value: "b"}}},
								},
							},
						},
						Link: "linknotfound.io/404",
					},
					{
						Junit: []JunitResult{
							{
								Result: junit.Result{
									Name:       "fake_test_1",
									ClassName:  "fake_class_0",
									Failure:    &failureMsgs[0],
									Properties: &junit.Properties{PropertyList: []junit.Property{{Name: "a", Value: "b"}}},
								},
							},
							{
								Result: junit.Result{
									Name:       "fake_test_1",
									ClassName:  "fake_class_0",
									Failure:    &failureMsgs[1],
									Properties: &junit.Properties{PropertyList: []junit.Property{{Name: "c", Value: "d"}}},
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
				Flaky: []TestResult{
					{
						Junit: []JunitResult{
							{
								junit.Result{
									Name:       "fake_test_2",
									ClassName:  "fake_class_0",
									Failure:    &failureMsgs[0],
									Properties: &junit.Properties{PropertyList: []junit.Property{{Name: "a", Value: "b"}}},
								},
							},
							{
								Result: junit.Result{
									Name:       "fake_test_2",
									ClassName:  "fake_class_0",
									Properties: &junit.Properties{PropertyList: []junit.Property{{Name: "c", Value: "d"}}},
								},
							},
						},
						Link: "linknotfound.io/404",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifacts := make([]api.Artifact, 0)
			for _, rr := range tt.rawResults {
				artifacts = append(artifacts, &FakeArtifact{
					path:      "log.txt",
					content:   rr,
					sizeLimit: 500e6,
				})
			}
			l := Lens{}
			got := l.getJvd(artifacts)
			if diff := cmp.Diff(tt.exp, got); diff != "" {
				t.Fatalf("JVD mismatch, want(-), got(+): \n%s", diff)
			}
		})
	}
}

func TestBody(t *testing.T) {
	for _, test := range []struct {
		name      string
		artifacts [][]byte
		expected  string
	}{
		{
			name: "Test-cases with properties",
			artifacts: [][]byte{
				[]byte(`
				<testsuites>
					<testsuite>
						<testcase classname="fake_class_0" name="fake_test_0">
							<failure message="Failed once" type=""> failure message 0 </failure>
							<properties>
								<property name="a" value="b"/>
							</properties>
						</testcase>
						<testcase classname="fake_class_0" name="fake_test_1">
							<failure message="Failed twice" type=""> failure message 0 </failure>
							<properties>
								<property name="a" value="b"/>
							</properties>
						</testcase>
						<testcase classname="fake_class_0" name="fake_test_1">
							<failure message="Failed twice" type=""> failure message 1 </failure>
							<properties>
								<property name="c" value="d"/>
							</properties>
						</testcase>
						<testcase classname="fake_class_0" name="fake_test_2">
							<failure message="Flaked once" type=""> failure message 0 </failure>
							<properties>
								<property name="a" value="b"/>
							</properties>
						</testcase>
						<testcase classname="fake_class_0" name="fake_test_2">
							<properties>
								<property name="c" value="d"/>
							</properties>
						</testcase>
					</testsuite>
				</testsuites>
				`),
			},
			expected: `





<div id="junit-container">
  <table id="junit-table" class="mdl-data-table mdl-js-data-table mdl-shadow--2dp">
  
  <tr id="failed-theader" class="header section-expander">
    <td class="mdl-data-table__cell--non-numeric expander failed" colspan="1"><h6>2/3 Tests Failed.</h6></td>
    <td class="mdl-data-table__cell--non-numeric expander"><i id="failed-expander" class="icon-button material-icons arrow-icon noselect">expand_less</i></td>
  </tr>
  <tbody id="failed-tbody">
    
      
      
      
      <tr>
        <td colspan="2" style="padding: 0;">
          <table class="failed-layout">
            <tr class="failure-name">
              <td class="mdl-data-table__cell--non-numeric test-name">fake_test_0&nbsp;<i class="icon-button material-icons arrow-icon">expand_more</i></td>
              <td class="mdl-data-table__cell--non-numeric" style="text-align: right;">0s</td>
            </tr>
            <tr class="hidden failure-text">
              <td colspan="2" class="mdl-data-table__cell--non-numeric">
                <dl>
                  <dt>a</dt>
                  <dd>b</dd>
                </dl>
                <div> failure message 0 </div>
                
              </td>
            </tr>
          </table>
        </td>
      </tr>
      
    
      
      
      
      <tr>
        <td colspan="2" style="padding: 0;">
          <table class="failed-layout">
            <tr class="failure-name">
              <td class="mdl-data-table__cell--non-numeric test-name">fake_test_1&nbsp;<i class="icon-button material-icons arrow-icon">expand_more</i></td>
            </tr>
            <tr class="hidden">
              <td>
                <table  class="failed-layout">
                  
                  <tr  class="failure-text">
                    <td colspan="2" style="padding: 0;">
                      <table class="failed-layout">
                        <tr class="failure-name">
                          <td class="mdl-data-table__cell--non-numeric test-name">Run #0: Failed&nbsp;<i class="icon-button material-icons arrow-icon">expand_more</i></td>
                          <td class="mdl-data-table__cell--non-numeric" style="text-align: right;">0s</td>
                        </tr>
                        <tr class="hidden failure-text">
                          <td colspan="2" class="mdl-data-table__cell--non-numeric">
                            <dl>
                              <dt>a</dt>
                              <dd>b</dd>
                            </dl>
                            <div> failure message 0 </div>
                            
                          </td>
                        </tr>
                      </table>
                    </td>
                  </tr>
                  
                  <tr  class="failure-text">
                    <td colspan="2" style="padding: 0;">
                      <table class="failed-layout">
                        <tr class="failure-name">
                          <td class="mdl-data-table__cell--non-numeric test-name">Run #1: Failed&nbsp;<i class="icon-button material-icons arrow-icon">expand_more</i></td>
                          <td class="mdl-data-table__cell--non-numeric" style="text-align: right;">0s</td>
                        </tr>
                        <tr class="hidden failure-text">
                          <td colspan="2" class="mdl-data-table__cell--non-numeric">
                            <dl>
                              <dt>c</dt>
                              <dd>d</dd>
                            </dl>
                            <div> failure message 1 </div>
                            
                          </td>
                        </tr>
                      </table>
                    </td>
                  </tr>
                  
                </table>
              </td>
            </tr>
          </table>
        </td>
      </tr>
      
    
  </tbody>
  
  
  <tr id="flaky-theader" class="header section-expander">
    <td class="mdl-data-table__cell--non-numeric expander flaky" colspan="1"><h6>1/3 Tests Flaky.</h6></td>
    <td class="mdl-data-table__cell--non-numeric expander"><i id="flaky-expander" class="icon-button material-icons arrow-icon noselect">expand_less</i></td>
  </tr>
  <tbody id="flaky-tbody">
    
      
      <tr>
        <td colspan="2" style="padding: 0;">
          <table class="flaky-layout">
            <tr class="flaky-name">
              <td class="mdl-data-table__cell--non-numeric test-name">fake_test_2&nbsp;<i class="icon-button material-icons arrow-icon">expand_more</i></td>
            </tr>
            <tr class="hidden">
              <td>
                <table class="flaky-layout">
                  
                  <tr  class="flaky-text">
                    <td colspan="2" style="padding: 0;">
                      <table class="flaky-layout">
                        <tr class="flaky-name">
                          <td class="mdl-data-table__cell--non-numeric test-name">Run #0: Failed&nbsp;<i class="icon-button material-icons arrow-icon">expand_more</i></td>
                          <td class="mdl-data-table__cell--non-numeric" style="text-align: right;">0s</td>
                        </tr>
                        <tr class="hidden flaky-text">
                          <td colspan="2" class="mdl-data-table__cell--non-numeric">
                            <dl>
                              <dt>a</dt>
                              <dd>b</dd>
                            </dl>
                            <div> failure message 0 </div>
                            
                          </td>
                        </tr>
                      </table>
                    </td>
                  </tr>
                  
                  <tr  class="flaky-text">
                    <td colspan="2" style="padding: 0;">
                      <table class="flaky-layout">
                        <tr class="flaky-name">
                          <td class="mdl-data-table__cell--non-numeric test-name">Run #1: Passed&nbsp;<i class="icon-button material-icons arrow-icon">expand_more</i></td>
                          <td class="mdl-data-table__cell--non-numeric" style="text-align: right;">0s</td>
                        </tr>
                        <tr class="hidden flaky-text">
                          <td colspan="2" class="mdl-data-table__cell--non-numeric">
                            <dl>
                              <dt>c</dt>
                              <dd>d</dd>
                            </dl>
                            <div>&lt;nil&gt;</div>
                            
                          </td>
                        </tr>
                      </table>
                    </td>
                  </tr>
                  
                </table>
              </td>
            </tr>
          </table>
        </td>
      </tr>
    
  </tbody>
  
  
  
  </table>
</div>

`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			artifacts := make([]api.Artifact, 0)
			for _, artifact := range test.artifacts {
				artifacts = append(artifacts, &FakeArtifact{
					path:      "log.txt",
					content:   artifact,
					sizeLimit: 500e6,
				})
			}
			lens := Lens{}
			got := lens.Body(artifacts, ".", "data", nil)
			if diff := cmp.Diff(test.expected, got); diff != "" {
				t.Fatalf("Body mismatch, want(-), got(+): \n%s", diff)
			}
		})
	}
}
