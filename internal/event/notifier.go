package event

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

const (
	// defaultNotifyTimeout caps each outbound Notify HTTP POST.
	defaultNotifyTimeout = 5 * time.Second

	// notifySOAPAction is the WS-BaseNotification Notify SOAPAction header value.
	notifySOAPAction = "http://docs.oasis-open.org/wsn/bw-2/NotificationConsumer/Notify"
)

// pushDispatch is one fan-out unit emitted by Broker.Publish for a single
// push subscription. Built under b.mu, then handed to the notifier goroutine.
type pushDispatch struct {
	subscriptionID   string
	subscriptionAddr string
	consumer         consumerEPR
	topic            string
	message          string
}

// notifier delivers Notify SOAP envelopes to push subscribers' ConsumerReference.
type notifier struct {
	client *http.Client
	logger *slog.Logger
}

func newNotifier(timeout time.Duration, logger *slog.Logger) *notifier {
	if timeout <= 0 {
		timeout = defaultNotifyTimeout
	}
	if logger == nil {
		logger = obs.Discard()
	}
	return &notifier{
		client: &http.Client{Timeout: timeout},
		logger: logger,
	}
}

// deliver POSTs a Notify SOAP envelope to the consumer. It calls onResult
// exactly once: true on HTTP 2xx, false otherwise (network error, timeout,
// non-2xx response). The body is fully drained before returning so the
// underlying TCP connection can be reused. Errors are logged at warn level;
// the caller (Broker) is responsible for failure-threshold bookkeeping.
func (n *notifier) deliver(ctx context.Context, d *pushDispatch, onResult func(success bool)) {
	body := buildNotifyEnvelope(d)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.consumer.address, bytes.NewReader(body))
	if err != nil {
		n.logger.Warn("event: notify build request",
			"subscription_id", d.subscriptionID, "consumer", d.consumer.address, "err", err)
		onResult(false)
		return
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("SOAPAction", notifySOAPAction)

	resp, err := n.client.Do(req)
	if err != nil {
		n.logger.Warn("event: notify dispatch",
			"subscription_id", d.subscriptionID, "consumer", d.consumer.address, "err", err)
		onResult(false)
		return
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			n.logger.Debug("event: notify body close", "err", cerr)
		}
	}()
	if _, copyErr := io.Copy(io.Discard, resp.Body); copyErr != nil {
		n.logger.Debug("event: notify body drain", "err", copyErr)
	}
	if resp.StatusCode/100 != 2 {
		n.logger.Warn("event: notify non-2xx",
			"subscription_id", d.subscriptionID,
			"consumer", d.consumer.address,
			"status", resp.StatusCode,
		)
		onResult(false)
		return
	}
	onResult(true)
}

// buildNotifyEnvelope renders a full SOAP 1.2 envelope wrapping a WS-BaseNotification
// Notify message. When d.consumer.referenceParams is non-empty its inner XML is
// echoed verbatim inside the SOAP Header (WS-Addressing requirement) so the
// consumer can route the Notify to the correct subscription on its side.
func buildNotifyEnvelope(d *pushDispatch) []byte {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString(`<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope"`)
	buf.WriteString(` xmlns:wsnt="`)
	buf.WriteString(eventsvc.WSNBaseNotificationNS)
	buf.WriteString(`"`)
	buf.WriteString(` xmlns:wsa="`)
	buf.WriteString(eventsvc.WSAddressingNamespace)
	buf.WriteString(`"`)
	buf.WriteString(` xmlns:tt="`)
	buf.WriteString(eventsvc.SchemaNamespace)
	buf.WriteString(`"`)
	buf.WriteString(` xmlns:tns1="http://www.onvif.org/ver10/topics">`)

	if d.consumer.referenceParams != "" {
		buf.WriteString(`<env:Header>`)
		buf.WriteString(d.consumer.referenceParams)
		buf.WriteString(`</env:Header>`)
	}

	buf.WriteString(`<env:Body>`)
	buf.WriteString(`<wsnt:Notify>`)
	buf.WriteString(`<wsnt:NotificationMessage>`)
	fmt.Fprintf(&buf,
		`<wsnt:SubscriptionReference><wsa:Address>%s</wsa:Address></wsnt:SubscriptionReference>`,
		xmlEscape(d.subscriptionAddr))
	fmt.Fprintf(&buf,
		`<wsnt:Topic Dialect="http://docs.oasis-open.org/wsn/t-1/TopicExpression/Concrete">%s</wsnt:Topic>`,
		xmlEscape(d.topic))
	buf.WriteString(`<wsnt:Message>`)
	buf.WriteString(d.message)
	buf.WriteString(`</wsnt:Message>`)
	buf.WriteString(`</wsnt:NotificationMessage>`)
	buf.WriteString(`</wsnt:Notify>`)
	buf.WriteString(`</env:Body>`)
	buf.WriteString(`</env:Envelope>`)
	return buf.Bytes()
}
