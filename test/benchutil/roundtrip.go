package benchutil

import (
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"k8s.io/client-go/rest"
)

// RTCounter counts HTTP round trips and bytes transferred.
type RTCounter struct {
	Cfg           *rest.Config
	count         atomic.Int64
	bytesSent     atomic.Int64
	bytesReceived atomic.Int64
}

// NewRTCounter creates a standalone counter. Use WrapTransport to inject it.
func NewRTCounter() *RTCounter {
	return &RTCounter{}
}

// TrackRoundTrips returns a config copy that counts round trips and bytes.
func TrackRoundTrips(cfg *rest.Config) *RTCounter {
	rc := NewRTCounter()
	rc.Cfg = rest.CopyConfig(cfg)
	rc.Cfg.Wrap(rc.WrapTransport)
	return rc
}

// ResetAll zeros counters and resets the benchmark timer.
func (rc *RTCounter) ResetAll(b *testing.B) {
	rc.reset()
	b.ResetTimer()
}

func (rc *RTCounter) RoundTrips() int64    { return rc.count.Load() }
func (rc *RTCounter) BytesSent() int64     { return rc.bytesSent.Load() }
func (rc *RTCounter) BytesReceived() int64 { return rc.bytesReceived.Load() }

// Report emits roundtrips/op, B-sent/op, and B-received/op.
func (rc *RTCounter) Report(b *testing.B) {
	n := float64(b.N)
	b.ReportMetric(float64(rc.RoundTrips())/n, "roundtrips/op")
	b.ReportMetric(float64(rc.BytesSent())/n, "B-sent/op")
	b.ReportMetric(float64(rc.BytesReceived())/n, "B-received/op")
}

// ReportAs emits metrics with a prefix (e.g. "remote-roundtrips/op").
func (rc *RTCounter) ReportAs(b *testing.B, prefix string) {
	n := float64(b.N)
	b.ReportMetric(float64(rc.RoundTrips())/n, prefix+"-roundtrips/op")
	b.ReportMetric(float64(rc.BytesSent())/n, prefix+"-B-sent/op")
	b.ReportMetric(float64(rc.BytesReceived())/n, prefix+"-B-received/op")
}

// WrapTransport returns a counting transport wrapper.
func (rc *RTCounter) WrapTransport(rt http.RoundTripper) http.RoundTripper {
	return &countingRoundTripper{
		delegate:      rt,
		count:         &rc.count,
		bytesSent:     &rc.bytesSent,
		bytesReceived: &rc.bytesReceived,
	}
}

func (rc *RTCounter) reset() {
	rc.count.Store(0)
	rc.bytesSent.Store(0)
	rc.bytesReceived.Store(0)
}

// countingRoundTripper wraps a transport to count round trips and bytes.
type countingRoundTripper struct {
	delegate      http.RoundTripper
	count         *atomic.Int64
	bytesSent     *atomic.Int64
	bytesReceived *atomic.Int64
}

func (c *countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.count.Add(1)

	// Count request body bytes.
	if req.Body != nil && req.Body != http.NoBody {
		req.Body = &countingReadCloser{ReadCloser: req.Body, count: c.bytesSent}
	}

	resp, err := c.delegate.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Wrap response body to count bytes as they're read.
	if resp.Body != nil {
		resp.Body = &countingReadCloser{ReadCloser: resp.Body, count: c.bytesReceived}
	}

	return resp, nil
}

// countingReadCloser wraps an io.ReadCloser to count bytes read.
type countingReadCloser struct {
	io.ReadCloser
	count *atomic.Int64
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	n, err := c.ReadCloser.Read(p)
	c.count.Add(int64(n))
	return n, err
}
