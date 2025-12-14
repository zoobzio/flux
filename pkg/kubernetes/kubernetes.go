// Package kubernetes provides a flux.Watcher implementation for Kubernetes
// ConfigMaps and Secrets using the Watch API.
package kubernetes

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

// ResourceType specifies the type of Kubernetes resource to watch.
type ResourceType int

const (
	// ConfigMap watches a ConfigMap resource.
	ConfigMap ResourceType = iota
	// Secret watches a Secret resource.
	Secret
)

// Watcher watches a Kubernetes ConfigMap or Secret for changes.
type Watcher struct {
	client       kubernetes.Interface
	namespace    string
	name         string
	key          string
	resourceType ResourceType
}

// Option configures a Watcher.
type Option func(*Watcher)

// WithResourceType sets the resource type to watch.
// Defaults to ConfigMap.
func WithResourceType(rt ResourceType) Option {
	return func(w *Watcher) {
		w.resourceType = rt
	}
}

// New creates a new Watcher for the given Kubernetes resource.
// The key specifies which data key within the ConfigMap/Secret to watch.
func New(client kubernetes.Interface, namespace, name, key string, opts ...Option) *Watcher {
	w := &Watcher{
		client:       client,
		namespace:    namespace,
		name:         name,
		key:          key,
		resourceType: ConfigMap,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Watch begins watching the Kubernetes resource and returns a channel that
// emits the key's value whenever it changes. The current value is emitted
// immediately to support initial configuration loading.
func (w *Watcher) Watch(ctx context.Context) (<-chan []byte, error) {
	out := make(chan []byte)

	go func() {
		defer close(out)

		for {
			if err := w.watchLoop(ctx, out); err != nil {
				if ctx.Err() != nil {
					return
				}
				// Reconnect on error
				continue
			}
			return
		}
	}()

	return out, nil
}

func (w *Watcher) watchLoop(ctx context.Context, out chan<- []byte) error {
	// Get initial value
	value, resourceVersion, err := w.getValue(ctx)
	if err != nil {
		return err
	}

	if value != nil {
		select {
		case out <- value:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Start watching
	opts := metav1.ListOptions{
		FieldSelector:   fmt.Sprintf("metadata.name=%s", w.name),
		ResourceVersion: resourceVersion,
		Watch:           true,
	}

	var watcher watch.Interface
	if w.resourceType == ConfigMap {
		watcher, err = w.client.CoreV1().ConfigMaps(w.namespace).Watch(ctx, opts)
	} else {
		watcher, err = w.client.CoreV1().Secrets(w.namespace).Watch(ctx, opts)
	}
	if err != nil {
		return fmt.Errorf("failed to start watch: %w", err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed")
			}

			if event.Type == watch.Error {
				return fmt.Errorf("watch error")
			}

			if event.Type == watch.Deleted {
				continue
			}

			value := w.extractValue(event.Object)
			if value != nil {
				select {
				case out <- value:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}

func (w *Watcher) getValue(ctx context.Context) ([]byte, string, error) {
	if w.resourceType == ConfigMap {
		cm, err := w.client.CoreV1().ConfigMaps(w.namespace).Get(ctx, w.name, metav1.GetOptions{})
		if err != nil {
			return nil, "", err
		}
		return []byte(cm.Data[w.key]), cm.ResourceVersion, nil
	}

	secret, err := w.client.CoreV1().Secrets(w.namespace).Get(ctx, w.name, metav1.GetOptions{})
	if err != nil {
		return nil, "", err
	}
	return secret.Data[w.key], secret.ResourceVersion, nil
}

func (w *Watcher) extractValue(obj interface{}) []byte {
	if w.resourceType == ConfigMap {
		if cm, ok := obj.(*corev1.ConfigMap); ok {
			return []byte(cm.Data[w.key])
		}
	} else {
		if secret, ok := obj.(*corev1.Secret); ok {
			return secret.Data[w.key]
		}
	}
	return nil
}
