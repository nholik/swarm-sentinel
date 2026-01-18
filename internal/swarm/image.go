package swarm

import "strings"

// NormalizeImage strips the @sha256:... digest suffix from a Docker image reference.
// Docker appends the resolved digest to image references after pulling, which can
// cause false mismatches when comparing desired vs actual images.
//
// Examples:
//   - "nginx:1.23@sha256:abc123..." → "nginx:1.23"
//   - "registry.example.com/app:v1@sha256:def456..." → "registry.example.com/app:v1"
//   - "nginx:1.23" → "nginx:1.23" (unchanged)
//   - "nginx@sha256:abc123..." → "nginx" (digest-only reference)
func NormalizeImage(image string) string {
	if idx := strings.Index(image, "@sha256:"); idx != -1 {
		return image[:idx]
	}
	return image
}
