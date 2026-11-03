package main

import (
	"strings"
	"testing"
)

func TestDecodeRunnerIdentityAcceptsUTF8BOM(t *testing.T) {
	expected := runnerIdentity{AgentID: 42, AgentName: "bookbuilder-runner", GitHubURL: "https://github.com/r14r/BookBuilder"}
	payload := []byte(`{"agentId":42,"agentName":"bookbuilder-runner","gitHubUrl":"https://github.com/r14r/BookBuilder"}`)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"normal", payload},
		{"BOM", append([]byte{0xEF, 0xBB, 0xBF}, payload...)},
		{"whitespace then BOM", append([]byte(" \r\n\t"), append([]byte{0xEF, 0xBB, 0xBF}, payload...)...)},
		{"BOM then whitespace", append([]byte{0xEF, 0xBB, 0xBF, ' ', '\n'}, payload...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeRunnerIdentityJSON(tc.data)
			if err != nil {
				t.Fatalf("valid runner identity rejected: %v", err)
			}
			if got != expected {
				t.Errorf("parsed %+v, want %+v", got, expected)
			}
		})
	}
}

func TestDecodeRunnerIdentityStillRejectsMalformedJSON(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("invalid JSON"),
		[]byte{0xEF, 0xBB, 0xBF, '{'},
		[]byte{0xEF, 0xBB, 0xBF, 0xEF, 0xBB, 0xBF, '{', '}'},
	} {
		_, err := decodeRunnerIdentityJSON(data)
		if err == nil {
			t.Errorf("malformed identity accepted: %q", data)
		}
	}
}

func TestDecodeRunnerIdentityDoesNotStripValidData(t *testing.T) {
	data := []byte(`{"agentId":9,"agentName":"x","gitHubUrl":"https://github.com/test/repo"}`)
	got, err := decodeRunnerIdentityJSON(data)
	if err != nil || got.AgentID != 9 || !strings.HasSuffix(got.GitHubURL, "/test/repo") {
		t.Fatalf("invalid decoded identity: %+v / %v", got, err)
	}
}
