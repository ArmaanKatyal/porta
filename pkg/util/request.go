package util

import "net/http"

// RequestToMap converts the request to a map
func RequestToMap(r *http.Request) map[string]any {
	result := make(map[string]any)

	result["method"] = r.Method

	result["url"] = r.URL.String()

	// Use the first value for each header, query parameter, and form field
	headers := make(map[string]string)
	for name, values := range r.Header {
		headers[name] = values[0]
	}
	result["headers"] = headers

	queryParams := make(map[string]string)
	for name, values := range r.URL.Query() {
		queryParams[name] = values[0]
	}
	result["query_params"] = queryParams

	if err := r.ParseForm(); err == nil {
		formValues := make(map[string]string)
		for name, values := range r.Form {
			formValues[name] = values[0]
		}
		result["form_values"] = formValues
	}

	return result
}
