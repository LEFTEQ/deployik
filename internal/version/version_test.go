package version

import "testing"

func TestNew(t *testing.T) {
	tests := []struct {
		name           string
		gitSHA         string
		buildTime      string
		ghRunID        string
		ghRepo         string
		wantShortSHA   string
		wantCommitURL  string
		wantRunURL     string
	}{
		{
			name:          "full release build",
			gitSHA:        "abc1234567890fedcba0123456789abcdef01234",
			buildTime:     "2026-04-19T10:23:11Z",
			ghRunID:       "12345678",
			ghRepo:        "lefteq/lovinka-deployik",
			wantShortSHA:  "abc1234",
			wantCommitURL: "https://github.com/lefteq/lovinka-deployik/commit/abc1234567890fedcba0123456789abcdef01234",
			wantRunURL:    "https://github.com/lefteq/lovinka-deployik/actions/runs/12345678",
		},
		{
			name:          "missing run id (PR build)",
			gitSHA:        "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			buildTime:     "2026-04-19T10:23:11Z",
			ghRunID:       "",
			ghRepo:        "lefteq/lovinka-deployik",
			wantShortSHA:  "deadbee",
			wantCommitURL: "https://github.com/lefteq/lovinka-deployik/commit/deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			wantRunURL:    "",
		},
		{
			name:          "local dev build",
			gitSHA:        "dev",
			buildTime:     "unknown",
			ghRunID:       "",
			ghRepo:        "lefteq/lovinka-deployik",
			wantShortSHA:  "dev",
			wantCommitURL: "",
			wantRunURL:    "",
		},
		{
			name:          "short sha shorter than 7",
			gitSHA:        "abc",
			buildTime:     "unknown",
			ghRunID:       "",
			ghRepo:        "lefteq/lovinka-deployik",
			wantShortSHA:  "abc",
			wantCommitURL: "",
			wantRunURL:    "",
		},
		{
			name:          "missing repo defeats both URLs",
			gitSHA:        "abc1234567890fedcba0123456789abcdef01234",
			buildTime:     "2026-04-19T10:23:11Z",
			ghRunID:       "12345678",
			ghRepo:        "",
			wantShortSHA:  "abc1234",
			wantCommitURL: "",
			wantRunURL:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := New(tc.gitSHA, tc.buildTime, tc.ghRunID, tc.ghRepo)
			if info.GitSHA != tc.wantShortSHA {
				t.Errorf("GitSHA: got %q, want %q", info.GitSHA, tc.wantShortSHA)
			}
			if info.GitSHAFull != tc.gitSHA {
				t.Errorf("GitSHAFull: got %q, want %q", info.GitSHAFull, tc.gitSHA)
			}
			if info.CommitURL != tc.wantCommitURL {
				t.Errorf("CommitURL: got %q, want %q", info.CommitURL, tc.wantCommitURL)
			}
			if info.RunURL != tc.wantRunURL {
				t.Errorf("RunURL: got %q, want %q", info.RunURL, tc.wantRunURL)
			}
			if info.GHRepo != tc.ghRepo {
				t.Errorf("GHRepo: got %q, want %q", info.GHRepo, tc.ghRepo)
			}
		})
	}
}

func TestIsDev(t *testing.T) {
	if !New("dev", "unknown", "", "lefteq/lovinka-deployik").IsDev() {
		t.Error("expected IsDev() == true for sha=\"dev\"")
	}
	if !New("", "unknown", "", "lefteq/lovinka-deployik").IsDev() {
		t.Error("expected IsDev() == true for empty sha")
	}
	if New("abc1234567890fedcba0123456789abcdef01234", "", "", "").IsDev() {
		t.Error("expected IsDev() == false for real sha")
	}
}
