package webhook

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/marco-spagn/pcmi/internal/crypto"
)

// Delivery is one outbound webhook HTTP POST.
type Delivery struct {
	ID        string
	URL       string
	Secret    string
	Body      []byte
	Timestamp int64
}

// NewDelivery builds a delivery with a fresh Unix timestamp when ts is zero.
func NewDelivery(id, url, secret string, body []byte, ts int64) Delivery {
	if ts == 0 {
		ts = time.Now().UTC().Unix()
	}
	return Delivery{
		ID:        id,
		URL:       url,
		Secret:    secret,
		Body:      body,
		Timestamp: ts,
	}
}

func (d Delivery) timestampString() string {
	return strconv.FormatInt(d.Timestamp, 10)
}

// ApplyHeaders sets standard PCMI webhook delivery headers on req.
func (d Delivery) ApplyHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PCMI-Event-Delivery", "1")
	req.Header.Set("X-PCMI-Delivery-ID", d.ID)
	req.Header.Set("X-PCMI-Timestamp", d.timestampString())
	if d.Secret != "" {
		req.Header.Set("X-PCMI-Signature", crypto.HMACSign(d.Secret, d.timestampString(), d.Body))
	}
}

// Post delivers the webhook body via HTTP POST.
func (d Delivery) Post(ctx context.Context, client *http.Client) error {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL, bytes.NewReader(d.Body))
	if err != nil {
		return err
	}
	d.ApplyHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
