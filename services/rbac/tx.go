package rbac

import (
	"context"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

type txKey struct{}

// WithTx returns a context that makes the Enterprise implementation read
// and write through tx, so its changes join the caller's transaction (and
// roll back with it). The licence seat guard wraps role changes this way.
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// TxFrom returns the transaction WithTx put in ctx, or nil.
func TxFrom(ctx context.Context) *gorm.DB {
	tx, _ := ctx.Value(txKey{}).(*gorm.DB)
	return tx
}
