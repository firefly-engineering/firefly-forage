package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
)

func TestGroupKey(t *testing.T) {
	tests := []struct {
		name string
		meta *config.SandboxMetadata
		want string
	}{
		{
			name: "uses SourceRepo when set",
			meta: &config.SandboxMetadata{
				SourceRepo: "/home/user/repo",
				Workspace:  "/var/lib/workspaces/sandbox1",
			},
			want: "/home/user/repo",
		},
		{
			name: "falls back to Workspace",
			meta: &config.SandboxMetadata{
				Workspace: "/home/user/project",
			},
			want: "/home/user/project",
		},
		{
			name: "empty SourceRepo uses Workspace",
			meta: &config.SandboxMetadata{
				SourceRepo: "",
				Workspace:  "/home/user/project",
			},
			want: "/home/user/project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupKey(tt.meta)
			if got != tt.want {
				t.Errorf("groupKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildGroupedItems(t *testing.T) {
	t.Run("empty sandboxes", func(t *testing.T) {
		items := buildGroupedItems(nil, nil)
		if items != nil {
			t.Errorf("expected nil, got %d items", len(items))
		}
	})

	t.Run("single group", func(t *testing.T) {
		sandboxes := []*config.SandboxMetadata{
			{Name: "sb1", Template: "claude", Workspace: "/home/user/project"},
			{Name: "sb2", Template: "aider", Workspace: "/home/user/project"},
		}
		items := buildGroupedItems(sandboxes, nil)

		// Expect 1 header + 2 sandbox items
		if len(items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(items))
		}

		// First item should be a header
		h, ok := items[0].(headerItem)
		if !ok {
			t.Fatal("first item should be a headerItem")
		}
		if h.label != "/home/user/project" {
			t.Errorf("header label = %q, want %q", h.label, "/home/user/project")
		}

		// Next two should be sandboxItems
		if _, ok := items[1].(sandboxItem); !ok {
			t.Error("second item should be a sandboxItem")
		}
		if _, ok := items[2].(sandboxItem); !ok {
			t.Error("third item should be a sandboxItem")
		}
	})

	t.Run("multiple groups sorted alphabetically", func(t *testing.T) {
		sandboxes := []*config.SandboxMetadata{
			{Name: "sb1", Template: "claude", SourceRepo: "/home/user/repo-b"},
			{Name: "sb2", Template: "aider", SourceRepo: "/home/user/repo-a"},
			{Name: "sb3", Template: "claude", SourceRepo: "/home/user/repo-b"},
		}
		items := buildGroupedItems(sandboxes, nil)

		// Expect 2 headers + 3 sandbox items = 5
		if len(items) != 5 {
			t.Fatalf("expected 5 items, got %d", len(items))
		}

		// First header should be repo-a (alphabetically first)
		h1, ok := items[0].(headerItem)
		if !ok {
			t.Fatal("first item should be a headerItem")
		}
		if h1.label != "/home/user/repo-a" {
			t.Errorf("first header = %q, want %q", h1.label, "/home/user/repo-a")
		}

		// Second header should be repo-b
		h2, ok := items[2].(headerItem)
		if !ok {
			t.Fatal("third item should be a headerItem")
		}
		if h2.label != "/home/user/repo-b" {
			t.Errorf("second header = %q, want %q", h2.label, "/home/user/repo-b")
		}
	})

	t.Run("mixed SourceRepo and Workspace grouping", func(t *testing.T) {
		sandboxes := []*config.SandboxMetadata{
			{Name: "sb1", Template: "claude", SourceRepo: "/home/user/repo", Workspace: "/var/lib/ws/sb1"},
			{Name: "sb2", Template: "aider", Workspace: "/home/user/project"},
		}
		items := buildGroupedItems(sandboxes, nil)

		// Expect 2 headers + 2 sandbox items = 4
		if len(items) != 4 {
			t.Fatalf("expected 4 items, got %d", len(items))
		}
	})
}

func TestHeaderItem(t *testing.T) {
	h := headerItem{label: "Test Group"}

	if h.FilterValue() != "" {
		t.Error("headerItem.FilterValue() should return empty string")
	}
	if h.Title() != "Test Group" {
		t.Errorf("Title() = %q, want %q", h.Title(), "Test Group")
	}
	if h.Description() != "" {
		t.Errorf("Description() = %q, want empty", h.Description())
	}
}

func TestHeaderCount(t *testing.T) {
	items := []list.Item{
		headerItem{label: "group1"},
		sandboxItem{metadata: &config.SandboxMetadata{Name: "sb1"}},
		sandboxItem{metadata: &config.SandboxMetadata{Name: "sb2"}},
		headerItem{label: "group2"},
		sandboxItem{metadata: &config.SandboxMetadata{Name: "sb3"}},
	}

	count := headerCount(items)
	if count != 2 {
		t.Errorf("headerCount() = %d, want 2", count)
	}
}

func TestShortenGroupKey(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/user/projects/myrepo", "projects/myrepo"},
		{"/tmp/test", "tmp/test"},
		{"short", "short"},
		{"a/b", "a/b"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := shortenGroupKey(tt.path)
			if got != tt.want {
				t.Errorf("shortenGroupKey(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
