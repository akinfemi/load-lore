package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/akinfemi/load-lore/internal/plan"
)

// seedVersion changes whenever the seeded table's shape or SQL changes, so
// old caches aren't reused.
const seedVersion = "1"

const seededMarker = ".loadlore-seeded"

// cacheVolume names the volume that caches a seeded dataset. The name is a
// hash of everything that changes the data on disk.
func cacheVolume(s *plan.Service) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%d|%s|%s", seedVersion, s.Image, s.Dataset.Rows, s.Dataset.RowBytes, strings.Join(s.Dataset.Indexes, ","), strings.Join(s.Dataset.SeedSQL(), ";"))
	return "loadlore-cache-pg-" + hex.EncodeToString(h.Sum(nil))[:16]
}

// prepareData makes a fresh writable data volume for a postgres service:
// a copy of the cached seeded volume, seeding it first on a cache miss.
// The copy keeps one run's writes out of the next run's data.
func (d *Driver) prepareData(ctx context.Context, s *plan.Service, progress func(string)) (string, error) {
	cache := cacheVolume(s)
	hit, err := d.volumeHasMarker(ctx, cache)
	if err != nil {
		return "", err
	}
	if !hit {
		progress(fmt.Sprintf("seeding %s: %d rows of about %d bytes (not cached yet)", s.Name, s.Dataset.Rows, s.Dataset.RowBytes))
		start := time.Now()
		if err := d.seed(ctx, s, cache); err != nil {
			return "", fmt.Errorf("seeding %s: %w", s.Name, err)
		}
		d.event("seed", map[string]any{"service": s.Name, "volume": cache, "rows": s.Dataset.Rows, "seconds": time.Since(start).Seconds()})
		progress(fmt.Sprintf("seeded %s in %s", s.Name, time.Since(start).Round(time.Second)))
	} else {
		d.event("seed_cache_hit", map[string]any{"service": s.Name, "volume": cache})
	}

	run := fmt.Sprintf("loadlore-%s-%s-data", d.runID, s.Name)
	progress(fmt.Sprintf("copying %s's cached data into a fresh volume", s.Name))
	start := time.Now()
	if _, err := d.docker(ctx, "volume", "create", "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID, run); err != nil {
		return "", err
	}
	if _, err := d.docker(ctx, "run", "--rm", "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID,
		"-v", cache+":/from:ro", "-v", run+":/to", ImageAlpine, "sh", "-c", "cp -a /from/. /to/ && rm -f /to/"+seededMarker); err != nil {
		return "", err
	}
	d.event("data_copy", map[string]any{"service": s.Name, "volume": run, "seconds": time.Since(start).Seconds()})
	return run, nil
}

func (d *Driver) volumeHasMarker(ctx context.Context, vol string) (bool, error) {
	out, err := d.docker(ctx, "volume", "ls", "-q", "--filter", "name=^"+vol+"$")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(string(out)) == "" {
		return false, nil
	}
	if _, err := d.docker(ctx, "run", "--rm", "--label", LabelOwner+"=1", "-v", vol+":/data:ro", ImageAlpine, "test", "-f", "/data/"+seededMarker); err != nil {
		// A volume without the marker is a seed that didn't finish.
		_, _ = d.docker(ctx, "volume", "rm", "-f", vol)
		return false, nil
	}
	return true, nil
}

// seed creates the cache volume, starts Postgres on it, runs the seed SQL
// inside the database (generate_series: no data crosses the network), then
// stops Postgres cleanly and marks the volume complete.
func (d *Driver) seed(ctx context.Context, s *plan.Service, vol string) error {
	name := fmt.Sprintf("loadlore-%s-seed-%s", d.runID, s.Name)
	if _, err := d.docker(ctx, "volume", "create", "--label", LabelOwner+"=1", "--label", LabelCache+"=dataset",
		"--label", fmt.Sprintf("io.loadlore.dataset=%d rows x %d bytes", s.Dataset.Rows, s.Dataset.RowBytes), vol); err != nil {
		return err
	}
	if _, err := d.docker(ctx, "run", "-d", "--name", name, "--label", LabelOwner+"=1", "--label", LabelRun+"="+d.runID,
		"--cpus", "2", "--memory", "2g", "--shm-size", "512m",
		"-e", "POSTGRES_USER="+plan.DBUser, "-e", "POSTGRES_PASSWORD="+plan.DBPassword, "-e", "POSTGRES_DB="+plan.DBName,
		"-v", vol+":"+pgDataPath(s.Version), s.Image,
		"postgres", "-c", "max_wal_size=4GB", "-c", "checkpoint_timeout=30min", "-c", "maintenance_work_mem=512MB"); err != nil {
		return err
	}
	defer func() { _, _ = d.docker(context.WithoutCancel(ctx), "rm", "-f", "-v", name) }()

	ready := false
	for i := 0; i < 120 && !ready; i++ {
		_, err := d.docker(ctx, "exec", name, "pg_isready", "-U", plan.DBUser, "-d", plan.DBName, "-h", "127.0.0.1")
		ready = err == nil
		if !ready {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
	if !ready {
		return fmt.Errorf("postgres didn't become ready")
	}
	sql := strings.Join(append(s.Dataset.SeedSQL(), "CHECKPOINT"), ";\n") + ";\n"
	if err := d.dockerStdin(ctx, sql, "exec", "-i", name, "psql", "-q", "-v", "ON_ERROR_STOP=1", "-U", plan.DBUser, "-d", plan.DBName); err != nil {
		return err
	}
	if _, err := d.docker(ctx, "stop", "-t", "120", name); err != nil {
		return err
	}
	_, err := d.docker(ctx, "run", "--rm", "--label", LabelOwner+"=1", "-v", vol+":/data", ImageAlpine, "touch", "/data/"+seededMarker)
	return err
}
