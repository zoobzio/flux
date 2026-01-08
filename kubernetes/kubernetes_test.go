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

func TestWithResourceType_SetsResourceType(t *testing.T) {
	client := fake.NewSimpleClientset()

	watcher := New(client, "default", "myconfig", "config.json")
	if watcher.resourceType != ConfigMap {
		t.Errorf("expected default ConfigMap, got %v", watcher.resourceType)
	}

	watcher = New(client, "default", "mysecret", "config.json", WithResourceType(Secret))
	if watcher.resourceType != Secret {
		t.Errorf("expected Secret, got %v", watcher.resourceType)
	}
}

func TestExtractValue_ConfigMap(t *testing.T) {
	client := fake.NewSimpleClientset()
	watcher := New(client, "default", "myconfig", "config.json")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myconfig",
			Namespace: "default",
		},
		Data: map[string]string{
			"config.json": `{"port": 8080}`,
		},
	}

	value := watcher.extractValue(cm)
	if string(value) != `{"port": 8080}` {
		t.Errorf("expected port 8080, got %q", value)
	}
}

func TestExtractValue_Secret(t *testing.T) {
	client := fake.NewSimpleClientset()
	watcher := New(client, "default", "mysecret", "config.json", WithResourceType(Secret))

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mysecret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"config.json": []byte(`{"api_key": "secret123"}`),
		},
	}

	value := watcher.extractValue(secret)
	if string(value) != `{"api_key": "secret123"}` {
		t.Errorf("expected api_key, got %q", value)
	}
}

func TestExtractValue_WrongType_ReturnsNil(t *testing.T) {
	client := fake.NewSimpleClientset()

	// ConfigMap watcher given a Secret
	watcher := New(client, "default", "myconfig", "config.json")
	secret := &corev1.Secret{
		Data: map[string][]byte{"config.json": []byte("data")},
	}
	if value := watcher.extractValue(secret); value != nil {
		t.Errorf("expected nil for wrong type, got %q", value)
	}

	// Secret watcher given a ConfigMap
	watcher = New(client, "default", "mysecret", "config.json", WithResourceType(Secret))
	cm := &corev1.ConfigMap{
		Data: map[string]string{"config.json": "data"},
	}
	if value := watcher.extractValue(cm); value != nil {
		t.Errorf("expected nil for wrong type, got %q", value)
	}
}

func TestExtractValue_InvalidObject_ReturnsNil(t *testing.T) {
	client := fake.NewSimpleClientset()
	watcher := New(client, "default", "myconfig", "config.json")

	// Pass a non-kubernetes object
	if value := watcher.extractValue("not a k8s object"); value != nil {
		t.Errorf("expected nil for invalid object, got %q", value)
	}
}

func TestGetValue_ConfigMap(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "myconfig",
			Namespace:       "default",
			ResourceVersion: "12345",
		},
		Data: map[string]string{
			"config.json": `{"port": 8080}`,
		},
	})

	watcher := New(client, "default", "myconfig", "config.json")
	value, rv, err := watcher.getValue(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(value) != `{"port": 8080}` {
		t.Errorf("expected port 8080, got %q", value)
	}
	if rv != "12345" {
		t.Errorf("expected resource version 12345, got %q", rv)
	}
}

func TestGetValue_Secret(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "mysecret",
			Namespace:       "default",
			ResourceVersion: "67890",
		},
		Data: map[string][]byte{
			"config.json": []byte(`{"api_key": "secret"}`),
		},
	})

	watcher := New(client, "default", "mysecret", "config.json", WithResourceType(Secret))
	value, rv, err := watcher.getValue(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(value) != `{"api_key": "secret"}` {
		t.Errorf("expected api_key, got %q", value)
	}
	if rv != "67890" {
		t.Errorf("expected resource version 67890, got %q", rv)
	}
}

func TestGetValue_NotFound(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	watcher := New(client, "default", "nonexistent", "config.json")
	_, _, err := watcher.getValue(ctx)

	if err == nil {
		t.Fatal("expected error for nonexistent ConfigMap")
	}

	watcher = New(client, "default", "nonexistent", "config.json", WithResourceType(Secret))
	_, _, err = watcher.getValue(ctx)

	if err == nil {
		t.Fatal("expected error for nonexistent Secret")
	}
}
