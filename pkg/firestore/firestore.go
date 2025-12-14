// Package firestore provides a flux.Watcher implementation for Firestore
// documents using realtime listeners.
package firestore

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
)

// Watcher watches a Firestore document for changes using realtime listeners.
type Watcher struct {
	client     *firestore.Client
	collection string
	document   string
	field      string
}

// Option configures a Watcher.
type Option func(*Watcher)

// WithField sets a specific field to extract from the document.
// If not set, the entire document is serialized as JSON.
func WithField(field string) Option {
	return func(w *Watcher) {
		w.field = field
	}
}

// New creates a new Watcher for the given Firestore document.
func New(client *firestore.Client, collection, document string, opts ...Option) *Watcher {
	w := &Watcher{
		client:     client,
		collection: collection,
		document:   document,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Watch begins watching the Firestore document and returns a channel that
// emits the document's data whenever it changes. The current value is emitted
// immediately to support initial configuration loading.
func (w *Watcher) Watch(ctx context.Context) (<-chan []byte, error) {
	docRef := w.client.Collection(w.collection).Doc(w.document)

	out := make(chan []byte)

	go func() {
		defer close(out)

		snapshots := docRef.Snapshots(ctx)
		defer snapshots.Stop()

		for {
			snap, err := snapshots.Next()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}

			if !snap.Exists() {
				continue
			}

			var value []byte
			if w.field != "" {
				// Extract specific field
				data := snap.Data()
				if fieldValue, ok := data[w.field]; ok {
					if bytes, ok := fieldValue.([]byte); ok {
						value = bytes
					} else if str, ok := fieldValue.(string); ok {
						value = []byte(str)
					}
				}
			} else {
				// Serialize entire document as JSON
				data := snap.Data()
				if dataField, ok := data["data"]; ok {
					if bytes, ok := dataField.([]byte); ok {
						value = bytes
					} else if str, ok := dataField.(string); ok {
						value = []byte(str)
					}
				}
			}

			if value == nil {
				continue
			}

			select {
			case out <- value:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

// CreateDocument is a helper to create a config document with the expected structure.
func CreateDocument(ctx context.Context, client *firestore.Client, collection, document string, data []byte) error {
	_, err := client.Collection(collection).Doc(document).Set(ctx, map[string]interface{}{
		"data": data,
	})
	if err != nil {
		return fmt.Errorf("failed to create document: %w", err)
	}
	return nil
}

// UpdateDocument is a helper to update a config document.
func UpdateDocument(ctx context.Context, client *firestore.Client, collection, document string, data []byte) error {
	_, err := client.Collection(collection).Doc(document).Update(ctx, []firestore.Update{
		{Path: "data", Value: data},
	})
	if err != nil {
		return fmt.Errorf("failed to update document: %w", err)
	}
	return nil
}
