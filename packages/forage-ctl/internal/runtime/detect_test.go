package runtime

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Type != RuntimeAuto {
		t.Errorf("expected RuntimeAuto, got %s", cfg.Type)
	}

	if cfg.ContainerPrefix != "forage-" {
		t.Errorf("expected 'forage-' prefix, got %s", cfg.ContainerPrefix)
	}
}

func TestAvailable(t *testing.T) {
	// Just ensure it doesn't panic
	available := Available()
	t.Logf("available runtimes: %v", available)
}

func TestDetect(t *testing.T) {
	// Just ensure it doesn't panic - actual result depends on system
	rt, err := Detect()
	if err != nil {
		t.Logf("no runtime detected (expected in minimal test env): %v", err)
	} else {
		t.Logf("detected runtime: %s", rt)
	}
}

func TestRuntimeTypes(t *testing.T) {
	tests := []struct {
		rt   RuntimeType
		want string
	}{
		{RuntimeNspawn, "nspawn"},
		{RuntimeDocker, "docker"},
		{RuntimePodman, "podman"},
		{RuntimeAuto, "auto"},
	}

	for _, tt := range tests {
		if string(tt.rt) != tt.want {
			t.Errorf("RuntimeType %v != %s", tt.rt, tt.want)
		}
	}
}
