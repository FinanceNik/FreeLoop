package config

import (
	"slices"
	"testing"
)

func TestWorktreeScripts(t *testing.T) {
	w, err := ParseWorktreeScripts([]byte("copy: [.env, config/local.json]\nsetup: npm install\nrun: npm run dev -- --port $FREELOOP_PORT\n"))
	if err != nil || w.Setup != "npm install" || !slices.Equal(w.Copy, []string{".env", "config/local.json"}) {
		t.Fatalf("parsed %+v, %v", w, err)
	}
	app := WorktreeScripts{Setup: "pnpm install", Archive: "docker compose down"}
	got := w.Over(app)
	if got.Setup != "pnpm install" || got.Run != w.Run || got.Archive != "docker compose down" || len(got.Copy) != 2 {
		t.Errorf("merged %+v", got)
	}
	for _, bad := range []string{"copy: [/etc/passwd]\n", "copy: [../secret]\n", "copy: [.]\n", "setpu: x\n"} {
		if _, err := ParseWorktreeScripts([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if !(WorktreeScripts{}).Empty() || w.Empty() {
		t.Error("Empty is wrong")
	}
}
