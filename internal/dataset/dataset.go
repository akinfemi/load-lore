// Package dataset defines the table Loadlore seeds into Postgres and the
// queries the synthetic service runs against it. Seeding happens inside the
// database with generate_series, so no data crosses the network.
package dataset

import (
	"fmt"
	"slices"
)

// Table is the seeded table's name.
const Table = "items"

// RowsPerUser sets how many rows share a user_id, so a user_id filter
// matches about this many rows.
const RowsPerUser = 10

// rowOverhead approximates the bytes of a row besides the payload (tuple
// header, id, user_id), so payload length ≈ row_bytes - rowOverhead.
const rowOverhead = 40

// Spec describes a dataset.
type Spec struct {
	Rows     int64
	RowBytes int
	Indexes  []string // extra indexed columns; id is always the primary key
}

// PayloadLen is the payload length that gives rows of about RowBytes.
func (s Spec) PayloadLen() int { return max(1, s.RowBytes-rowOverhead) }

// Users is the number of distinct user_id values.
func (s Spec) Users() int64 { return max(1, s.Rows/RowsPerUser) }

// UserFor maps a key to a user_id, for scans that filter on user_id.
func (s Spec) UserFor(key int64) int64 { return (key-1)%s.Users() + 1 }

// SeedSQL returns the statements that create and fill the table. The
// payload is stored uncompressed (STORAGE EXTERNAL) so the table's size on
// disk matches row_bytes rather than shrinking under TOAST compression.
func (s Spec) SeedSQL() []string {
	n := s.PayloadLen()
	stmts := []string{
		fmt.Sprintf(`DROP TABLE IF EXISTS %s`, Table),
		fmt.Sprintf(`CREATE TABLE %s (id bigint NOT NULL, user_id bigint NOT NULL, payload text NOT NULL)`, Table),
		fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN payload SET STORAGE EXTERNAL`, Table),
		fmt.Sprintf(`INSERT INTO %s (id, user_id, payload)
SELECT g, ((g - 1) %% %d) + 1, repeat(md5(g::text), %d) || left(md5(g::text), %d)
FROM generate_series(1, %d) AS g`, Table, s.Users(), n/32, n%32, s.Rows),
		fmt.Sprintf(`ALTER TABLE %s ADD PRIMARY KEY (id)`, Table),
	}
	idx := slices.Clone(s.Indexes)
	slices.Sort(idx)
	for _, col := range slices.Compact(idx) {
		if col == "id" {
			continue
		}
		stmts = append(stmts, fmt.Sprintf(`CREATE INDEX %s_%s_idx ON %s (%s)`, Table, col, Table, col))
	}
	return append(stmts, fmt.Sprintf(`VACUUM ANALYZE %s`, Table))
}

// Queries the synthetic service runs. $1 is always derived from the request key.
const (
	// PointRead fetches one row by primary key.
	PointRead = `SELECT payload FROM items WHERE id = $1`
	// RangeRead fetches $2 rows starting at an id, through the primary key.
	RangeRead = `SELECT id, payload FROM items WHERE id >= $1 ORDER BY id LIMIT $2`
	// BatchRead fetches the rows whose ids are in $1, through the primary
	// key. Unlike a range, the planner needs no estimate of where in the
	// index the keys fall, so the plan is generic and cached.
	BatchRead = `SELECT id, payload FROM items WHERE id = ANY($1)`
	// ScanRead filters on user_id: a full table scan unless user_id is indexed.
	ScanRead = `SELECT count(*), max(length(payload)) FROM items WHERE user_id = $1`
	// Write updates one row's payload by primary key.
	Write = `UPDATE items SET payload = $2 WHERE id = $1`
)
