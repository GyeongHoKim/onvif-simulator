package eventsvc_test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

// ---------- stub providers -------------------------------------------------------

type stubProvider struct{}

func (stubProvider) EventServiceCapabilities(context.Context) (eventsvc.ServiceCapabilities, error) {
	return eventsvc.ServiceCapabilities{
		WSPullPointSupport: true,
		MaxPullPoints:      10,
	}, nil
}

func (stubProvider) EventProperties(context.Context) (eventsvc.EventProperties, error) {
	return eventsvc.EventProperties{
		FixedTopicSet: true,
		TopicSet:      `<tns1:VideoSource xmlns:tns1="http://www.onvif.org/onvif/ver10/topics"/>`,
	}, nil
}

func (stubProvider) CreatePullPointSubscription(
	_ context.Context, _ eventsvc.CreatePullPointSubscriptionParams,
) (eventsvc.SubscriptionInfo, error) {
	return eventsvc.SubscriptionInfo{
		SubscriptionID:  "sub-001",
		TerminationTime: time.Now().Add(time.Hour),
		CurrentTime:     time.Now(),
	}, nil
}

func (stubProvider) PullMessages(
	_ context.Context, _ string, _ eventsvc.PullMessagesParams,
) (eventsvc.PullMessagesResult, error) {
	return eventsvc.PullMessagesResult{
		CurrentTime:     time.Now(),
		TerminationTime: time.Now().Add(time.Hour),
		Messages:        []eventsvc.NotificationMessage{},
	}, nil
}

func (stubProvider) SetSynchronizationPoint(_ context.Context, _ string) error { return nil }

func (stubProvider) Renew(
	_ context.Context, _ string, _ eventsvc.RenewParams,
) (eventsvc.RenewResult, error) {
	return eventsvc.RenewResult{
		TerminationTime: time.Now().Add(time.Hour),
		CurrentTime:     time.Now(),
	}, nil
}

func (stubProvider) Unsubscribe(_ context.Context, _ string) error { return nil }

// ---------- SOAP envelope builders -----------------------------------------------

func envelopeForEvent(op string) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body><tev:` + op + ` xmlns:tev="` + eventsvc.EventsNamespace + `"/></s:Body></s:Envelope>`
}

func envelopeForWSN(op string) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body><wsnt:` + op + ` xmlns:wsnt="` + eventsvc.WSNBaseNotificationNS + `"/></s:Body></s:Envelope>`
}

// ---------- auth handler builders ------------------------------------------------

func newEventAuthenticatedHandler(t *testing.T, users ...auth.UserRecord) http.Handler {
	t.Helper()
	store := auth.NewMutableUserStore(users)
	digest := auth.NewDigestAuthenticator(store, auth.DigestOptions{Realm: "onvif"})
	ut := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{})
	hook := auth.NewOperationAuthorizer(
		auth.NewChain(digest, ut),
		auth.DefaultPolicy(),
		auth.MapOperationClass(auth.EventOperationClasses),
	)
	return eventsvc.NewEventServiceHandler(stubProvider{}, eventsvc.WithEventAuthHook(
		eventsvc.AuthFunc(hook),
	))
}

func newSubscriptionAuthenticatedHandler(t *testing.T, users ...auth.UserRecord) http.Handler {
	t.Helper()
	store := auth.NewMutableUserStore(users)
	digest := auth.NewDigestAuthenticator(store, auth.DigestOptions{Realm: "onvif"})
	ut := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{})
	hook := auth.NewOperationAuthorizer(
		auth.NewChain(digest, ut),
		auth.DefaultPolicy(),
		auth.MapOperationClass(auth.EventOperationClasses),
	)
	return eventsvc.NewSubscriptionManagerHandler(stubProvider{}, eventsvc.WithSubscriptionManagerAuthHook(
		eventsvc.AuthFunc(hook),
	))
}

// ---------- digest helper --------------------------------------------------------

func buildDigestHeader(
	t *testing.T, username, password, uri, nonce string,
) string {
	t.Helper()
	const nc = "00000001"
	const cnonce = "c"
	h := md5.New()
	hasher := func(s string) string {
		h.Reset()
		h.Write([]byte(s))
		return hex.EncodeToString(h.Sum(nil))
	}
	ha1 := hasher(username + ":onvif:" + password)
	ha2 := hasher("POST:" + uri)
	resp := hasher(strings.Join([]string{ha1, nonce, nc, cnonce, "auth", ha2}, ":"))
	return strings.Join([]string{
		`Digest username="` + username + `"`,
		`realm="onvif"`,
		`nonce="` + nonce + `"`,
		`uri="` + uri + `"`,
		`qop=auth`,
		`nc=` + nc,
		`cnonce="` + cnonce + `"`,
		`response="` + resp + `"`,
		`algorithm=MD5`,
	}, ", ")
}
