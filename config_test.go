package main

import (
	"os"
	"regexp"
	"testing"
	"time"
)

func lookupFrom(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func TestLoadConfig_WithoutTheControlPlane_Fails(t *testing.T) {
	for _, values := range []map[string]string{
		{},
		{"CP_BASE_URL": "http://cp"},
		{"CP_INTERNAL_TOKEN": "t"},
	} {
		if _, err := loadConfig(lookupFrom(values)); err == nil {
			t.Errorf("%v: started without the control plane", values)
		}
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	loaded, err := loadConfig(lookupFrom(map[string]string{"CP_BASE_URL": "http://cp", "CP_INTERNAL_TOKEN": "t"}))

	if err != nil {
		t.Fatal(err)
	}
	if loaded.pollInterval != 2*time.Second || loaded.reconcileInterval != 2*time.Minute ||
		loaded.imageServer != "https://images.linuxcontainers.org" || loaded.healthAddr != ":8080" {
		t.Fatalf("defaults: %+v", loaded)
	}
}

func TestLoadConfig_BadDuration_Fails(t *testing.T) {
	_, err := loadConfig(lookupFrom(map[string]string{"CP_BASE_URL": "http://cp", "CP_INTERNAL_TOKEN": "t", "POLL_INTERVAL": "soon"}))

	if err == nil {
		t.Fatal("accepted POLL_INTERVAL=soon")
	}
}

// The control plane's kinds, read from its C# source as copied by scripts/sync-kinds.sh rather than retyped: a kind
// added there and not routed here would stay Pending forever
// (https://github.com/OpenViCloud/titan-ops/blob/main/docs/architecture/provider-and-worker.md#provider-command-table).
const providerCommandKindsSource = "contract/ProviderCommandKinds.cs"

var kindConstant = regexp.MustCompile(`public const string \w+ = "([^"]+)";`)

func controlPlaneKinds(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile(providerCommandKindsSource)
	if err != nil {
		t.Fatalf("read %s: %v", providerCommandKindsSource, err)
	}
	kinds := []string{}
	for _, match := range kindConstant.FindAllStringSubmatch(string(source), -1) {
		kinds = append(kinds, match[1])
	}
	if len(kinds) == 0 {
		t.Fatalf("no kind constant found in %s", providerCommandKindsSource)
	}
	return kinds
}

// A route with no kind yet is allowed: the worker ships a new kind before the control plane enqueues it, and keeps a
// removed one until the control plane has stopped
// (https://github.com/OpenViCloud/titan-ops/blob/main/docs/architecture/provider-and-worker.md#contract-changes).
func TestRoutes_CoverTheControlPlanesKinds(t *testing.T) {
	routes := routes(nil, nil, "")
	kinds := controlPlaneKinds(t)

	for _, kind := range kinds {
		route, found := routes[kind]
		if !found || route.Timeout <= 0 {
			t.Errorf("%s: no route, or a route without a timeout", kind)
		}
	}
}
