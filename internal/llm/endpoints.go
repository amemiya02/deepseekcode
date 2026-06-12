package llm

import "strings"

// APIEndpoint builds a versioned API endpoint from a provider base URL.
// It accepts both service roots (https://api.example.com) and OpenAI-style
// version roots (https://api.example.com/v1).
func APIEndpoint(baseURL, path string) string {
	base := strings.TrimRight(baseURL, "/")
	cleanPath := strings.Trim(path, "/")
	if cleanPath == "" {
		return base
	}
	if base == "" {
		return "/v1/" + cleanPath
	}
	if strings.HasSuffix(base, "/"+cleanPath) {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/" + cleanPath
	}
	return base + "/v1/" + cleanPath
}

func ChatCompletionsEndpoint(baseURL string) string {
	return APIEndpoint(baseURL, "chat/completions")
}

func ModelsEndpoint(baseURL string) string {
	return APIEndpoint(baseURL, "models")
}

func MessagesEndpoint(baseURL string) string {
	return APIEndpoint(baseURL, "messages")
}
