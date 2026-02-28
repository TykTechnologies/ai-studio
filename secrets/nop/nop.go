// Package nop provides a no-op implementation of secrets.SecretStore.
// It registers itself under the name "nop" via init().
// Values pass through unencrypted, and CRUD operations are no-ops.
package nop

import (
	"context"
	"fmt"
	"sync"

	"github.com/TykTechnologies/midsommar/v2/secrets"
)

func init() {
	secrets.RegisterStore("nop", func(_ interface{}, _ string) (secrets.SecretStore, error) {
		return New(), nil
	})
}

// Nop implements secrets.SecretStore as a no-op.
// Encrypt/Decrypt pass through values unchanged.
// CRUD operations succeed silently without persisting anything.
type Nop struct {
	mu      sync.RWMutex
	store   map[uint]*secrets.Secret
	byName  map[string]uint
	nextID  uint
}

// New creates a new no-op secret store.
func New() *Nop {
	return &Nop{
		store:  make(map[uint]*secrets.Secret),
		byName: make(map[string]uint),
		nextID: 1,
	}
}

func (n *Nop) Create(_ context.Context, secret *secrets.Secret) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	secret.ID = n.nextID
	n.nextID++
	cp := *secret
	n.store[secret.ID] = &cp
	n.byName[secret.VarName] = secret.ID
	return nil
}

func (n *Nop) GetByID(_ context.Context, id uint, preserveRef bool) (*secrets.Secret, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	s, ok := n.store[id]
	if !ok {
		return nil, fmt.Errorf("nop: secret %d not found", id)
	}
	cp := *s
	if preserveRef {
		cp.PreserveReference()
	}
	return &cp, nil
}

func (n *Nop) GetByVarName(_ context.Context, name string, preserveRef bool) (*secrets.Secret, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	id, ok := n.byName[name]
	if !ok {
		return nil, fmt.Errorf("nop: secret %q not found", name)
	}
	s := n.store[id]
	cp := *s
	if preserveRef {
		cp.PreserveReference()
	}
	return &cp, nil
}

func (n *Nop) Update(_ context.Context, secret *secrets.Secret) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.store[secret.ID]; !ok {
		return fmt.Errorf("nop: secret %d not found", secret.ID)
	}
	cp := *secret
	n.store[secret.ID] = &cp
	n.byName[secret.VarName] = secret.ID
	return nil
}

func (n *Nop) Delete(_ context.Context, id uint) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	s, ok := n.store[id]
	if !ok {
		return nil
	}
	delete(n.byName, s.VarName)
	delete(n.store, id)
	return nil
}

func (n *Nop) List(_ context.Context, pageSize, pageNumber int, all bool) ([]secrets.Secret, int64, int, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	var items []secrets.Secret
	for _, s := range n.store {
		items = append(items, *s)
	}
	total := int64(len(items))
	totalPages := int(total) / pageSize
	if int(total)%pageSize != 0 {
		totalPages++
	}
	if !all && len(items) > pageSize {
		start := (pageNumber - 1) * pageSize
		end := start + pageSize
		if start > len(items) {
			items = nil
		} else if end > len(items) {
			items = items[start:]
		} else {
			items = items[start:end]
		}
	}
	return items, total, totalPages, nil
}

func (n *Nop) EnsureDefaults(ctx context.Context, names []string) error {
	for _, name := range names {
		n.mu.RLock()
		_, exists := n.byName[name]
		n.mu.RUnlock()
		if !exists {
			if err := n.Create(ctx, &secrets.Secret{VarName: name, Value: ""}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n *Nop) EncryptValue(_ context.Context, plaintext string) (string, error) {
	return plaintext, nil
}

func (n *Nop) DecryptValue(_ context.Context, ciphertext string) (string, error) {
	return ciphertext, nil
}

func (n *Nop) ResolveReference(_ context.Context, reference string, _ bool) string {
	return reference
}

func (n *Nop) RotateKey(_ context.Context, _, _ string) (*secrets.RotationResult, error) {
	return &secrets.RotationResult{}, nil
}
