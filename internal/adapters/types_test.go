package adapters

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestParseErrorResponse_OpenAIErrorShape(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body: io.NopCloser(strings.NewReader(`{
			"error": {
				"message": "invalid model",
				"type": "invalid_request_error",
				"code": "model_not_found"
			}
		}`)),
	}

	err := ParseErrorResponse(resp)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Message != "invalid model" {
		t.Fatalf("unexpected message: %s", apiErr.Message)
	}
	if apiErr.Type != "invalid_request_error" {
		t.Fatalf("unexpected type: %s", apiErr.Type)
	}
	if apiErr.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", apiErr.HTTPStatus)
	}
	if apiErr.Code != "model_not_found" {
		t.Fatalf("unexpected code: %#v", apiErr.Code)
	}
}

func TestParseErrorResponse_Fallback(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       io.NopCloser(strings.NewReader("bad gateway")),
	}

	err := ParseErrorResponse(resp)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.HTTPStatus != http.StatusBadGateway {
		t.Fatalf("unexpected status: %d", apiErr.HTTPStatus)
	}
	if apiErr.Type != "upstream_error" {
		t.Fatalf("unexpected type: %s", apiErr.Type)
	}
	if !strings.Contains(apiErr.Message, "upstream HTTP 502") {
		t.Fatalf("unexpected message: %s", apiErr.Message)
	}
}
