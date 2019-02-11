package rest

import (
	"net/http"
)

func WriteHTTPStatus(w http.ResponseWriter, statusCode int) {
	http.Error(w, http.StatusText(statusCode), statusCode)
}
