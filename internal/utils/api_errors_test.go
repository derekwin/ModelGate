package utils

import (
	"testing"
)

func TestErrorResponse(t *testing.T) {
	err := ErrorResponse("test error message")

	if err.Message != "test error message" {
		t.Errorf("ErrorResponse() Message = %s, want %s", err.Message, "test error message")
	}

	if err.Type != "invalid_request_error" {
		t.Errorf("ErrorResponse() Type = %s, want %s", err.Type, "invalid_request_error")
	}

	if err.Code != 400 {
		t.Errorf("ErrorResponse() Code = %d, want %d", err.Code, 400)
	}
}

func TestValidationError(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		detail string
		want   APIError
	}{
		{
			name:   "field and detail",
			field:  "username",
			detail: "is required",
			want:   APIError{Message: "username: is required", Type: "validation_error", Code: 422},
		},
		{
			name:   "field only",
			field:  "email",
			detail: "",
			want:   APIError{Message: "email", Type: "validation_error", Code: 422},
		},
		{
			name:   "detail only",
			field:  "",
			detail: "invalid format",
			want:   APIError{Message: "invalid format", Type: "validation_error", Code: 422},
		},
		{
			name:   "both empty",
			field:  "",
			detail: "",
			want:   APIError{Message: "validation error", Type: "validation_error", Code: 422},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidationError(tt.field, tt.detail)
			if err.Message != tt.want.Message {
				t.Errorf("ValidationError() Message = %s, want %s", err.Message, tt.want.Message)
			}
			if err.Type != tt.want.Type {
				t.Errorf("ValidationError() Type = %s, want %s", err.Type, tt.want.Type)
			}
			if err.Code != tt.want.Code {
				t.Errorf("ValidationError() Code = %d, want %d", err.Code, tt.want.Code)
			}
		})
	}
}
