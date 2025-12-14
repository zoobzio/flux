package integration

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zoobzio/flux"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCapacitor_Kubernetes_ConfigMap_InitialLoad(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := appConfig{Feature: "k8s-test", Limit: 300}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myconfig",
			Namespace: "default",
		},
		Data: map[string]string{
			"config.json": string(data),
		},
	})

	// Create a simple channel-based watcher that emits on ConfigMap changes
	watcher := &fakeK8sWatcher{
		client:    client,
		namespace: "default",
		name:      "myconfig",
		key:       "config.json",
	}

	var applied appConfig

	capacitor := flux.New[appConfig](
		watcher,
		func(_ context.Context, _, cfg appConfig) error {
			applied = cfg
			return nil
		},
	)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if capacitor.State() != flux.StateHealthy {
		t.Errorf("expected StateHealthy, got %s", capacitor.State())
	}

	if applied.Feature != "k8s-test" || applied.Limit != 300 {
		t.Errorf("unexpected applied config: %+v", applied)
	}
}

func TestCapacitor_Kubernetes_Secret_InitialLoad(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := appConfig{Feature: "secret-test", Limit: 400}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	client := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mysecret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"config.json": data,
		},
	})

	watcher := &fakeK8sSecretWatcher{
		client:    client,
		namespace: "default",
		name:      "mysecret",
		key:       "config.json",
	}

	var applied appConfig

	capacitor := flux.New[appConfig](
		watcher,
		func(_ context.Context, _, cfg appConfig) error {
			applied = cfg
			return nil
		},
	)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if capacitor.State() != flux.StateHealthy {
		t.Errorf("expected StateHealthy, got %s", capacitor.State())
	}

	if applied.Feature != "secret-test" || applied.Limit != 400 {
		t.Errorf("unexpected applied config: %+v", applied)
	}
}

func TestCapacitor_Kubernetes_ConfigMap_LiveUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := appConfig{Feature: "v1", Limit: 10}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myconfig",
			Namespace: "default",
		},
		Data: map[string]string{
			"config.json": string(data),
		},
	})

	updateCh := make(chan []byte, 1)
	watcher := &fakeK8sWatcher{
		client:    client,
		namespace: "default",
		name:      "myconfig",
		key:       "config.json",
		updateCh:  updateCh,
	}

	var applyCount atomic.Int32
	var lastApplied atomic.Value

	capacitor := flux.New[appConfig](
		watcher,
		func(_ context.Context, _, cfg appConfig) error {
			applyCount.Add(1)
			lastApplied.Store(cfg)
			return nil
		},
	).Debounce(50 * time.Millisecond)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if applyCount.Load() != 1 {
		t.Errorf("expected 1 apply after start, got %d", applyCount.Load())
	}

	// Simulate update
	cfg2 := appConfig{Feature: "v2", Limit: 20}
	data2, err := json.Marshal(cfg2)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	updateCh <- data2

	// Wait for debounced apply
	if !waitFor(t, time.Second, func() bool { return applyCount.Load() == 2 }) {
		t.Fatalf("expected 2 applies, got %d", applyCount.Load())
	}

	applied := lastApplied.Load().(appConfig)
	if applied.Feature != "v2" || applied.Limit != 20 {
		t.Errorf("unexpected applied config: %+v", applied)
	}
}

func TestCapacitor_Kubernetes_InvalidUpdateRetainsPrevious(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := appConfig{Feature: "valid", Limit: 50}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myconfig",
			Namespace: "default",
		},
		Data: map[string]string{
			"config.json": string(data),
		},
	})

	updateCh := make(chan []byte, 1)
	watcher := &fakeK8sWatcher{
		client:    client,
		namespace: "default",
		name:      "myconfig",
		key:       "config.json",
		updateCh:  updateCh,
	}

	capacitor := flux.New[appConfig](
		watcher,
		func(_ context.Context, _, _ appConfig) error {
			return nil
		},
	).Debounce(50 * time.Millisecond)

	if err := capacitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Send invalid config (limit -1 violates min=0)
	invalidCfg := appConfig{Feature: "invalid", Limit: -1}
	invalidData, err := json.Marshal(invalidCfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	updateCh <- invalidData

	// Wait for processing
	if !waitFor(t, time.Second, func() bool { return capacitor.State() == flux.StateDegraded }) {
		t.Fatalf("expected StateDegraded, got %s", capacitor.State())
	}

	// Previous config should still be current
	current, ok := capacitor.Current()
	if !ok {
		t.Fatal("expected current config to exist")
	}
	if current.Feature != "valid" || current.Limit != 50 {
		t.Errorf("expected previous config retained, got %+v", current)
	}

	if capacitor.LastError() == nil {
		t.Error("expected LastError to be set")
	}
}

// fakeK8sWatcher implements flux.Watcher for testing with Kubernetes ConfigMaps.
type fakeK8sWatcher struct {
	client    *fake.Clientset
	namespace string
	name      string
	key       string
	updateCh  chan []byte
}

func (w *fakeK8sWatcher) Watch(ctx context.Context) (<-chan []byte, error) {
	out := make(chan []byte)

	go func() {
		defer close(out)

		// Emit initial value
		cm, err := w.client.CoreV1().ConfigMaps(w.namespace).Get(ctx, w.name, metav1.GetOptions{})
		if err != nil {
			return
		}

		select {
		case out <- []byte(cm.Data[w.key]):
		case <-ctx.Done():
			return
		}

		// If no update channel, return after initial value
		if w.updateCh == nil {
			<-ctx.Done()
			return
		}

		// Watch for updates
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-w.updateCh:
				if !ok {
					return
				}
				select {
				case out <- data:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}

// fakeK8sSecretWatcher implements flux.Watcher for testing with Kubernetes Secrets.
type fakeK8sSecretWatcher struct {
	client    *fake.Clientset
	namespace string
	name      string
	key       string
	updateCh  chan []byte
}

func (w *fakeK8sSecretWatcher) Watch(ctx context.Context) (<-chan []byte, error) {
	out := make(chan []byte)

	go func() {
		defer close(out)

		// Emit initial value
		secret, err := w.client.CoreV1().Secrets(w.namespace).Get(ctx, w.name, metav1.GetOptions{})
		if err != nil {
			return
		}

		select {
		case out <- secret.Data[w.key]:
		case <-ctx.Done():
			return
		}

		// If no update channel, return after initial value
		if w.updateCh == nil {
			<-ctx.Done()
			return
		}

		// Watch for updates
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-w.updateCh:
				if !ok {
					return
				}
				select {
				case out <- data:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, nil
}
