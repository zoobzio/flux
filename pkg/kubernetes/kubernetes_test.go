package kubernetes

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWatcher_EmitsInitialValue_ConfigMap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myconfig",
			Namespace: "default",
		},
		Data: map[string]string{
			"config.json": `{"port": 8080}`,
		},
	})

	watcher := New(client, "default", "myconfig", "config.json")
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	select {
	case data := <-ch:
		if string(data) != `{"port": 8080}` {
			t.Errorf("expected port 8080, got %q", data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for initial value")
	}
}

func TestWatcher_EmitsInitialValue_Secret(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mysecret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"config.json": []byte(`{"api_key": "secret123"}`),
		},
	})

	watcher := New(client, "default", "mysecret", "config.json", WithResourceType(Secret))
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	select {
	case data := <-ch:
		if string(data) != `{"api_key": "secret123"}` {
			t.Errorf("expected api_key, got %q", data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for initial value")
	}
}

func TestWatcher_ClosesOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myconfig",
			Namespace: "default",
		},
		Data: map[string]string{
			"config.json": `{"port": 8080}`,
		},
	})

	watcher := New(client, "default", "myconfig", "config.json")
	ch, err := watcher.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Drain initial
	<-ch

	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to close")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for channel close")
	}
}
