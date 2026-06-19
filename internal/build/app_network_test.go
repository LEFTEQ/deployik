package build

import "testing"

func TestAppNetworkName(t *testing.T) {
	cases := []struct {
		appID string
		env   string
		want  string
	}{
		{"01ABC", "preview", "deployik-app-01ABC-preview"},
		{"01ABC", "production", "deployik-app-01ABC-production"},
	}
	for _, c := range cases {
		if got := AppNetworkName(c.appID, c.env); got != c.want {
			t.Errorf("AppNetworkName(%q,%q) = %q, want %q", c.appID, c.env, got, c.want)
		}
	}
	// Per-(app,env) scoping: preview and production are distinct networks.
	if AppNetworkName("x", "preview") == AppNetworkName("x", "production") {
		t.Fatal("preview and production app networks must differ")
	}
}
