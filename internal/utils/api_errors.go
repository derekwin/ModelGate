package utils

type APIError struct {
    Message string `json:"message"`
    Type    string `json:"type"`
    Code    int    `json:"code"`
}

func ErrorResponse(message string) APIError {
    return APIError{
        Message: message,
        Type:    "invalid_request_error",
        Code:    400,
    }
}

func ValidationError(field string, detail string) APIError {
    var msg string
    if field != "" && detail != "" {
        msg = field + ": " + detail
    } else if field != "" {
        msg = field
    } else {
        msg = detail
    }
    if msg == "" {
        msg = "validation error"
    }
    return APIError{
        Message: msg,
        Type:    "validation_error",
        Code:    422,
    }
}
