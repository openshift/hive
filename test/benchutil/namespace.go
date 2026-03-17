package benchutil

import (
	"context"
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
)

// BenchNamespace creates a unique namespace for benchmark isolation.
func BenchNamespace(b *testing.B, cfg *rest.Config) string {
	b.Helper()
	benchClientOnce.Do(func() {
		cs, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create clientset: %v\n", err)
			os.Exit(1)
		}
		benchClientset = cs
	})
	name := fmt.Sprintf("bench-%d", benchNSCounter.Add(1))
	_, err := benchClientset.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	if err != nil {
		b.Fatalf("failed to create namespace %s: %v", name, err)
	}
	return name
}
