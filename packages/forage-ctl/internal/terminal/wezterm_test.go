package terminal

import "testing"

func TestVersionSupportsControlMode(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		// Unstable format
		{
			name:    "unstable before cutoff",
			version: "0-unstable-2025-03-07",
			want:    false,
		},
		{
			name:    "unstable on cutoff",
			version: "0-unstable-2025-03-08",
			want:    true,
		},
		{
			name:    "unstable after cutoff",
			version: "0-unstable-2025-04-01",
			want:    true,
		},
		// Stable format
		{
			name:    "stable before cutoff",
			version: "20250307-120000-abcdef0",
			want:    false,
		},
		{
			name:    "stable after cutoff",
			version: "20250309-080000-1a2b3c4",
			want:    true,
		},
		{
			name:    "stable on cutoff",
			version: "20250308-000000-0000000",
			want:    true,
		},
		// Edge cases
		{
			name:    "empty string",
			version: "",
			want:    false,
		},
		{
			name:    "garbage string",
			version: "not-a-version",
			want:    false,
		},
		{
			name:    "whitespace only",
			version: "   ",
			want:    false,
		},
		{
			name:    "unstable far future",
			version: "0-unstable-2026-01-15",
			want:    true,
		},
		{
			name:    "stable year before cutoff",
			version: "20240101-120000-abcdef0",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := versionSupportsControlMode(tt.version)
			if got != tt.want {
				t.Errorf("versionSupportsControlMode(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
