package store

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// tstamp 把 time.Time 包成 pgtype.Timestamptz(Valid=true)。
func tstamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
