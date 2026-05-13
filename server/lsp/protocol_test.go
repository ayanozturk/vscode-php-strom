package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResponseMessageMarshalsNullResult(t *testing.T) {
	id := json.RawMessage(`1`)
	data, err := json.Marshal(ResponseMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Result:  nil,
	})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	jsonText := string(data)
	if !strings.Contains(jsonText, `"result":null`) {
		t.Fatalf("expected null result in response, got %s", jsonText)
	}
	if strings.Contains(jsonText, `"error"`) {
		t.Fatalf("did not expect error field in success response, got %s", jsonText)
	}
}
