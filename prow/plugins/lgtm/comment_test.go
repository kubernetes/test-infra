package lgtm

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func Test_parseValidLGTMFromimelines(t *testing.T) {
	tests := []struct {
		name        string
		commentBody string
		want        []string
	}{
		{
			name:        "no timelines",
			commentBody: lgtmTimelineNotificationHeader,
			want:        nil,
		},
		{
			name: "one lgtm timelines",
			commentBody: strings.Join([]string{
				lgtmTimelineNotificationHeader,
				stringifyLgtmTimelineRecordLine(time.Now(), true, "user1"),
			}, "\n"),
			want: []string{"user1"},
		},
		{
			name: "multi lgtm timelines",
			commentBody: strings.Join([]string{
				lgtmTimelineNotificationHeader,
				stringifyLgtmTimelineRecordLine(time.Now(), true, "user1"),
				stringifyLgtmTimelineRecordLine(time.Now(), true, "user2"),
			}, "\n"),
			want: []string{"user1", "user2"},
		},
		{
			name: "have a latest reset vote in timelines",
			commentBody: strings.Join([]string{
				lgtmTimelineNotificationHeader,
				stringifyLgtmTimelineRecordLine(time.Now(), true, "user1"),
				stringifyLgtmTimelineRecordLine(time.Now(), true, "user2"),
				stringifyLgtmTimelineRecordLine(time.Now(), false, "user3"),
			}, "\n"),
			want: []string{},
		},
		{
			name: "have a previous reset vote in timelines",
			commentBody: strings.Join([]string{
				lgtmTimelineNotificationHeader,
				stringifyLgtmTimelineRecordLine(time.Now(), true, "user1"),
				stringifyLgtmTimelineRecordLine(time.Now(), true, "user2"),
				stringifyLgtmTimelineRecordLine(time.Now(), false, "user3"),
				stringifyLgtmTimelineRecordLine(time.Now(), true, "user4"),
			}, "\n"),
			want: []string{"user4"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseValidLGTMFromTimelines(tt.commentBody); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseValidLGTMFromimelines() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseLgtmTimelineRecordLine(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		want  bool
		want1 string
	}{
		{
			name:  "lgtm",
			line:  stringifyLgtmTimelineRecordLine(time.Now(), true, "fakeUser"),
			want:  true,
			want1: "fakeUser",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := parseLgtmTimelineRecordLine(tt.line)
			if got != tt.want {
				t.Errorf("parseLgtmTimelineRecordLine() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("parseLgtmTimelineRecordLine() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
