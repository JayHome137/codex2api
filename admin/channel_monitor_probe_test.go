package admin

import (
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func TestReadChannelMonitorSSERequiresTerminalAndExtractsText(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"type":"reasoning","summary":[]}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"go"}`,
		"",
		`data: {"type":"response.completed","response":{"status":"completed"}}`,
		"",
	}, "\n")
	result := readChannelMonitorSSE(strings.NewReader(stream), time.Now().Add(-100*time.Millisecond))
	if result.Failure != "" || !result.Terminal || result.Text != "go" || result.FirstTokenMS <= 0 {
		t.Fatalf("SSE result = %#v", result)
	}
}

func TestReadChannelMonitorJSONFindsMessageAfterReasoning(t *testing.T) {
	body := []byte(`{
		"status":"completed",
		"output":[
			{"type":"reasoning","summary":[]},
			{"type":"message","content":[{"type":"output_text","text":"go"}]}
		]
	}`)
	result := readChannelMonitorJSON(body)
	if result.Failure != "" || !result.Terminal || result.Text != "go" || result.FirstTokenMS != 0 {
		t.Fatalf("JSON result = %#v", result)
	}
}

func TestChannelMonitorAnswerMatchesExpectedWord(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"go", true},
		{"`go`", true},
		{"GO!", true},
		{"going", false},
		{"The answer is go.", false},
		{"no number", false},
	} {
		if got := channelMonitorAnswerMatches(tc.text, "go"); got != tc.want {
			t.Fatalf("channelMonitorAnswerMatches(%q) = %t, want %t", tc.text, got, tc.want)
		}
	}
}

func TestBuildChannelMonitorPayloadUsesNonStreamingResponses(t *testing.T) {
	payload, expected := buildChannelMonitorPayload("gpt-monitor", 7)
	if expected == "" || gjson.GetBytes(payload, "model").String() != "gpt-monitor" ||
		gjson.GetBytes(payload, "stream").Bool() || gjson.GetBytes(payload, "store").Bool() {
		t.Fatalf("payload = %s, expected=%q", payload, expected)
	}
	if got := gjson.GetBytes(payload, "max_output_tokens").Int(); got != 64 {
		t.Fatalf("max_output_tokens = %d, want 64", got)
	}
}
