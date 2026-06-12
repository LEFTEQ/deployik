package push

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/LEFTEQ/lovinka-deployik/internal/db"
)

type sentPush struct {
	endpoint string
	payload  string
}

type mockSender struct {
	mu     sync.Mutex
	status map[string]int // endpoint -> status to return (default 201)
	sent   []sentPush
}

func (m *mockSender) Send(sub *webpush.Subscription, payload []byte, _ *webpush.Options) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentPush{endpoint: sub.Endpoint, payload: string(payload)})
	if status, ok := m.status[sub.Endpoint]; ok {
		return status, nil
	}
	return 201, nil
}

func (m *mockSender) sentEndpoints() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	endpoints := make([]string, len(m.sent))
	for i, s := range m.sent {
		endpoints[i] = s.endpoint
	}
	return endpoints
}

func setupDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func createUser(t *testing.T, database *db.DB, githubID int64, username string) *db.User {
	t.Helper()
	user := &db.User{ID: db.NewID(), GithubID: githubID, Username: username, Role: "user"}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	return user
}

func createProject(t *testing.T, database *db.DB, userID string) *db.Project {
	t.Helper()
	project := &db.Project{
		Name:        "push-test-" + db.NewID()[20:],
		GithubRepo:  "repo",
		GithubOwner: "owner",
		Branch:      "main",
		UserID:      userID,
		Framework:   "nextjs",
		Status:      "active",
	}
	if err := database.CreateProject(project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	return project
}

func subscribe(t *testing.T, database *db.DB, userID, endpoint string, prefs [3]bool) *db.PushSubscription {
	t.Helper()
	sub := &db.PushSubscription{
		UserID:               userID,
		Endpoint:             endpoint,
		P256dh:               "p256dh-key",
		Auth:                 "auth-key",
		NotifyDeployOutcomes: prefs[0],
		NotifyBuildStarts:    prefs[1],
		NotifySSLIssues:      prefs[2],
	}
	if err := database.UpsertPushSubscription(sub); err != nil {
		t.Fatalf("upsert subscription: %v", err)
	}
	return sub
}

func newTestNotifier(database *db.DB, sender Sender) (*Notifier, *sync.WaitGroup) {
	wg := &sync.WaitGroup{}
	return &Notifier{
		DB:      database,
		Keys:    &VAPIDKeys{PublicKey: "pub", PrivateKey: "priv"},
		Subject: "mailto:test@example.com",
		Sender:  sender,
		Wg:      wg,
	}, wg
}

func TestNotifySendsToOwnerAndHonorsToggles(t *testing.T) {
	database := setupDB(t)
	owner := createUser(t, database, 1, "owner")
	project := createProject(t, database, owner.ID)

	// Device A wants deploy outcomes; device B has them muted.
	subscribe(t, database, owner.ID, "https://push.example/a", [3]bool{true, true, true})
	subscribe(t, database, owner.ID, "https://push.example/b", [3]bool{false, true, true})

	sender := &mockSender{}
	notifier, wg := newTestNotifier(database, sender)

	notifier.Notify(project.ID, EventDeployOutcome, Message{
		Title: "Deployment live",
		Body:  "production is live",
		URL:   "/projects/" + project.ID,
	})
	wg.Wait()

	endpoints := sender.sentEndpoints()
	if len(endpoints) != 1 || endpoints[0] != "https://push.example/a" {
		t.Fatalf("expected exactly device A, got %v", endpoints)
	}

	var msg Message
	if err := json.Unmarshal([]byte(sender.sent[0].payload), &msg); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if msg.Title != "Deployment live" || msg.URL != "/projects/"+project.ID {
		t.Fatalf("unexpected payload: %+v", msg)
	}
}

func TestNotifyExcludesForeignUsers(t *testing.T) {
	database := setupDB(t)
	owner := createUser(t, database, 1, "owner")
	stranger := createUser(t, database, 2, "stranger")
	project := createProject(t, database, owner.ID)

	subscribe(t, database, owner.ID, "https://push.example/owner", [3]bool{true, true, true})
	subscribe(t, database, stranger.ID, "https://push.example/stranger", [3]bool{true, true, true})

	sender := &mockSender{}
	notifier, wg := newTestNotifier(database, sender)

	notifier.Notify(project.ID, EventBuildStart, Message{Title: "Build started"})
	wg.Wait()

	endpoints := sender.sentEndpoints()
	if len(endpoints) != 1 || endpoints[0] != "https://push.example/owner" {
		t.Fatalf("stranger must not be notified, got %v", endpoints)
	}
}

func TestNotifyDeletesGoneSubscriptions(t *testing.T) {
	database := setupDB(t)
	owner := createUser(t, database, 1, "owner")
	project := createProject(t, database, owner.ID)

	gone := subscribe(t, database, owner.ID, "https://push.example/gone", [3]bool{true, true, true})

	sender := &mockSender{status: map[string]int{gone.Endpoint: 410}}
	notifier, wg := newTestNotifier(database, sender)

	notifier.Notify(project.ID, EventSSLIssue, Message{Title: "SSL failed"})
	wg.Wait()

	stored, err := database.GetPushSubscriptionByEndpoint(gone.Endpoint)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if stored != nil {
		t.Fatal("410 Gone subscription should have been deleted")
	}
}

func TestNotifyNilNotifierIsNoop(t *testing.T) {
	var notifier *Notifier
	// Must not panic.
	notifier.Notify("some-project", EventDeployOutcome, Message{Title: "x"})
}

func TestLoadOrCreateVAPIDKeysPersists(t *testing.T) {
	dir := t.TempDir()

	first, err := LoadOrCreateVAPIDKeys(dir)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if first.PublicKey == "" || first.PrivateKey == "" {
		t.Fatal("generated keys are empty")
	}

	second, err := LoadOrCreateVAPIDKeys(dir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if second.PublicKey != first.PublicKey || second.PrivateKey != first.PrivateKey {
		t.Fatal("keys must be stable across restarts")
	}

	info, err := os.Stat(filepath.Join(dir, "vapid.json"))
	if err != nil {
		t.Fatalf("stat vapid.json: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("vapid.json mode = %v, want 0600", info.Mode().Perm())
	}
}
