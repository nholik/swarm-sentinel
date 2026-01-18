package compose

import (
	"context"
	"strings"
	"testing"
)

func TestParseDesiredState_Basic(t *testing.T) {
	composeYAML := `
services:
  web:
    image: nginx:1.23
    deploy:
      replicas: 3
  worker:
    image: busybox:latest
`

	state, err := ParseDesiredState(context.Background(), []byte(composeYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	web, ok := state.Services["web"]
	if !ok {
		t.Fatalf("expected web service")
	}
	if web.Image != "nginx:1.23" {
		t.Fatalf("unexpected web image: %q", web.Image)
	}
	if web.Mode != defaultDeployMode {
		t.Fatalf("unexpected web mode: %q", web.Mode)
	}
	if web.Replicas != 3 {
		t.Fatalf("unexpected web replicas: %d", web.Replicas)
	}

	worker, ok := state.Services["worker"]
	if !ok {
		t.Fatalf("expected worker service")
	}
	if worker.Replicas != 1 {
		t.Fatalf("unexpected worker replicas: %d", worker.Replicas)
	}
}

func TestParseDesiredState_GlobalMode(t *testing.T) {
	composeYAML := `
services:
  node:
    image: busybox:latest
    deploy:
      mode: global
`

	state, err := ParseDesiredState(context.Background(), []byte(composeYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	service := state.Services["node"]
	if service.Mode != globalDeployMode {
		t.Fatalf("unexpected mode: %q", service.Mode)
	}
	if service.Replicas != 0 {
		t.Fatalf("unexpected replicas for global mode: %d", service.Replicas)
	}
}

func TestParseDesiredState_ConfigSecretNormalization(t *testing.T) {
	composeYAML := `
name: prod
services:
  api:
    image: example/api:1
    configs:
      - app_config
      - source: shared_config
        target: /etc/shared
    secrets:
      - db_password
      - source: api_secret
        target: /run/secrets/api
configs:
  app_config:
    external: true
    name: app_config_v3
  shared_config:
    file: ./shared.conf
secrets:
  db_password:
    external: true
    name: db_password_v2
  api_secret:
    file: ./secret.txt
`

	state, err := ParseDesiredState(context.Background(), []byte(composeYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	api := state.Services["api"]
	if got, want := strings.Join(api.Configs, ","), "app_config_v3,prod_shared_config"; got != want {
		t.Fatalf("unexpected configs: %s", got)
	}
	if got, want := strings.Join(api.Secrets, ","), "db_password_v2,prod_api_secret"; got != want {
		t.Fatalf("unexpected secrets: %s", got)
	}
}

func TestParseDesiredState_MissingImage(t *testing.T) {
	composeYAML := `
services:
  app:
    build: .
`

	_, err := ParseDesiredState(context.Background(), []byte(composeYAML))
	if err == nil || !strings.Contains(err.Error(), "missing image") {
		t.Fatalf("expected missing image error, got %v", err)
	}
}

func TestParseDesiredState_InvalidYAML(t *testing.T) {
	_, err := ParseDesiredState(context.Background(), []byte("services: ["))
	if err == nil {
		t.Fatalf("expected error for invalid yaml")
	}
}

func TestParseDesiredState_NoServices(t *testing.T) {
	_, err := ParseDesiredState(context.Background(), []byte("services: {}"))
	if err == nil || !strings.Contains(err.Error(), "no services") {
		t.Fatalf("expected no services error, got %v", err)
	}
}
