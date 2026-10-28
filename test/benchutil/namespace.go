package benchutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	benchNSCounter  atomic.Int64
	benchClientset  *kubernetes.Clientset
	benchClientOnce sync.Once
	// benchNSPrefix makes namespace names unique across runs, which matters
	// in real-cluster mode: previous runs' namespaces may still be
	// Terminating (deletion is asynchronous, and never completes at all on a
	// bare API server without the namespace controller), and a name stuck in
	// Terminating cannot be recreated.
	benchNSPrefix string
)

func init() {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Degenerate environments without crypto/rand: fall back to the
		// process ID, which still separates same-host runs. (A nanosecond
		// timestamp would be a worse fallback: rendered in hex, its leading
		// digits -- all a fixed-width slice can take -- barely change
		// between runs.)
		b = []byte(fmt.Sprintf("%x", os.Getpid()))
	}
	benchNSPrefix = "bench-" + hex.EncodeToString(b)
}

// BenchNamespace creates a unique namespace for benchmark isolation and
// deletes it when the benchmark completes. The delete round trips go through
// the uncounted clientset, so they do not pollute benchmark metrics.
func BenchNamespace(b *testing.B, cfg *rest.Config) string {
	b.Helper()
	benchClientOnce.Do(func() {
		cs, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			b.Fatalf("failed to create clientset: %v", err)
		}
		benchClientset = cs
	})
	name := fmt.Sprintf("%s-%d", benchNSPrefix, benchNSCounter.Add(1))
	_, err := benchClientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	if err != nil {
		b.Fatalf("failed to create namespace %s: %v", name, err)
	}
	b.Cleanup(func() {
		if err := benchClientset.CoreV1().Namespaces().Delete(context.Background(), name, metav1.DeleteOptions{}); err != nil {
			b.Logf("failed to delete namespace %s: %v", name, err)
		}
	})
	return name
}
