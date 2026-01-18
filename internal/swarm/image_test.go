package swarm

import "testing"

func TestNormalizeImage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "image with tag and digest",
			input: "nginx:1.23@sha256:abc123def456",
			want:  "nginx:1.23",
		},
		{
			name:  "full registry path with digest",
			input: "registry.example.com/myapp/api:v2.1.0@sha256:0123456789abcdef",
			want:  "registry.example.com/myapp/api:v2.1.0",
		},
		{
			name:  "image without digest",
			input: "nginx:1.23",
			want:  "nginx:1.23",
		},
		{
			name:  "image with latest tag",
			input: "busybox:latest",
			want:  "busybox:latest",
		},
		{
			name:  "digest only reference",
			input: "nginx@sha256:abc123def456",
			want:  "nginx",
		},
		{
			name:  "image without tag or digest",
			input: "nginx",
			want:  "nginx",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "full sha256 digest",
			input: "myregistry.io/app:prod@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			want:  "myregistry.io/app:prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizeImage(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeImage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
