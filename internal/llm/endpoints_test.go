package llm

import "testing"

func TestAPIEndpointAcceptsRootOrVersionedBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		path string
		want string
	}{
		{
			name: "service root",
			base: "https://api.example.com",
			path: "chat/completions",
			want: "https://api.example.com/v1/chat/completions",
		},
		{
			name: "version root",
			base: "https://api.example.com/v1",
			path: "chat/completions",
			want: "https://api.example.com/v1/chat/completions",
		},
		{
			name: "version root with trailing slash",
			base: "https://api.example.com/v1/",
			path: "/chat/completions",
			want: "https://api.example.com/v1/chat/completions",
		},
		{
			name: "complete endpoint",
			base: "https://api.example.com/v1/chat/completions",
			path: "chat/completions",
			want: "https://api.example.com/v1/chat/completions",
		},
		{
			name: "models endpoint",
			base: "https://api.example.com/v1",
			path: "models",
			want: "https://api.example.com/v1/models",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := APIEndpoint(tt.base, tt.path); got != tt.want {
				t.Fatalf("APIEndpoint(%q, %q) = %q, want %q", tt.base, tt.path, got, tt.want)
			}
		})
	}
}
