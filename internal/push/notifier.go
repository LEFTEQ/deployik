package push

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/lefteq/lovinka-deployik/internal/db"
)

// EventType selects the per-device preference gate for a notification.
type EventType string

const (
	EventDeployOutcome EventType = "deploy_outcome"
	EventBuildStart    EventType = "build_start"
	EventSSLIssue      EventType = "ssl_issue"
)

func (e EventType) preferenceColumn() string {
	switch e {
	case EventDeployOutcome:
		return "notify_deploy_outcomes"
	case EventBuildStart:
		return "notify_build_starts"
	case EventSSLIssue:
		return "notify_ssl_issues"
	}
	return ""
}

// Message is the payload delivered to the service worker. URL is an in-app
// path the notificationclick handler deep-links to.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	// Tag collapses successive notifications for the same subject (e.g. one
	// per deployment) instead of stacking them.
	Tag string `json:"tag,omitempty"`
}

// Sender delivers one encrypted push message. Implemented by webpush-go in
// production and by a mock in tests.
type Sender interface {
	Send(subscription *webpush.Subscription, payload []byte, options *webpush.Options) (statusCode int, err error)
}

type webpushSender struct{}

func (webpushSender) Send(subscription *webpush.Subscription, payload []byte, options *webpush.Options) (int, error) {
	resp, err := webpush.SendNotification(payload, subscription, options)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

// Notifier fans a project event out to every eligible device. All sends are
// best-effort: errors are logged, never returned to the caller, and a send
// can never block or fail a deployment.
type Notifier struct {
	DB      *db.DB
	Keys    *VAPIDKeys
	Subject string // mailto: or https: URL identifying the sender (VAPID sub claim)
	Sender  Sender
	// Wg, when set, tracks in-flight notification goroutines so the server
	// can drain them on shutdown (same pattern as pipeline screenshots).
	Wg *sync.WaitGroup
}

// NewNotifier wires the production webpush sender.
func NewNotifier(database *db.DB, keys *VAPIDKeys, subject string, wg *sync.WaitGroup) *Notifier {
	return &Notifier{DB: database, Keys: keys, Subject: subject, Sender: webpushSender{}, Wg: wg}
}

// Notify sends msg to every subscribed device of every user with access to
// the project, honoring per-device event toggles. Asynchronous and nil-safe:
// a nil Notifier (push disabled) is a no-op.
func (n *Notifier) Notify(projectID string, event EventType, msg Message) {
	if n == nil || n.DB == nil || n.Keys == nil || n.Sender == nil {
		return
	}
	column := event.preferenceColumn()
	if column == "" {
		log.Printf("Push: unknown event type %q", event)
		return
	}

	subs, err := n.DB.ListPushSubscriptionsForProject(projectID, column)
	if err != nil {
		log.Printf("Push: failed to list subscriptions for project %s: %v", projectID, err)
		return
	}
	if len(subs) == 0 {
		return
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Push: failed to marshal payload: %v", err)
		return
	}

	if n.Wg != nil {
		n.Wg.Add(1)
	}
	go func() {
		if n.Wg != nil {
			defer n.Wg.Done()
		}
		for _, sub := range subs {
			n.deliver(&sub, payload)
		}
	}()
}

func (n *Notifier) deliver(sub *db.PushSubscription, payload []byte) {
	status, err := n.Sender.Send(&webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, payload, &webpush.Options{
		Subscriber:      n.Subject,
		VAPIDPublicKey:  n.Keys.PublicKey,
		VAPIDPrivateKey: n.Keys.PrivateKey,
		TTL:             3600,
		Urgency:         webpush.UrgencyNormal,
	})
	if err != nil {
		log.Printf("Push: delivery to subscription %s failed: %v", sub.ID, err)
		if dbErr := n.DB.MarkPushSubscriptionFailed(sub.Endpoint); dbErr != nil {
			log.Printf("Push: %v", dbErr)
		}
		return
	}

	switch status {
	case http.StatusNotFound, http.StatusGone:
		// The push service says this subscription no longer exists.
		if err := n.DB.DeletePushSubscriptionByEndpoint(sub.Endpoint); err != nil {
			log.Printf("Push: %v", err)
		}
	case http.StatusCreated, http.StatusOK, http.StatusAccepted:
		// Delivered.
	default:
		log.Printf("Push: unexpected status %d for subscription %s", status, sub.ID)
		if err := n.DB.MarkPushSubscriptionFailed(sub.Endpoint); err != nil {
			log.Printf("Push: %v", err)
		}
	}
}
