// Command bootstrap-pgbin fetches and extracts the same embedded-postgres
// binaries cmd/engine bundles for offline first-launch (see
// postgresBinariesPath in cmd/engine/main.go) into a plain directory,
// without needing a running database — it starts one just long enough to
// force the library's own download+extract, then stops it immediately.
//
// Used by the release workflow (.github/workflows/release.yml) to
// regenerate engine/assets/pgbin-windows-amd64 on every CI run (that
// directory isn't committed to git — see the root .gitignore — since it's
// ~100MB of third-party binaries CI can always fetch fresh), and can be
// re-run locally the same way if that cached copy ever needs refreshing.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func main() {
	out := flag.String("out", "", "directory to extract the Postgres binaries into (required)")
	flag.Parse()

	if *out == "" {
		log.Fatal("--out is required")
	}
	absOut, err := filepath.Abs(*out)
	if err != nil {
		log.Fatalf("resolve --out: %v", err)
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		log.Fatalf("create --out: %v", err)
	}

	tmpData, err := os.MkdirTemp("", "bootstrap-pgbin-data")
	if err != nil {
		log.Fatalf("temp data dir: %v", err)
	}
	defer os.RemoveAll(tmpData)
	tmpRuntime, err := os.MkdirTemp("", "bootstrap-pgbin-runtime")
	if err != nil {
		log.Fatalf("temp runtime dir: %v", err)
	}
	defer os.RemoveAll(tmpRuntime)

	// Same Postgres version as cmd/engine — must match, since the two are
	// meant to produce the identical binary set.
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V16).
		Port(15432).
		Username("bootstrap").
		Password("bootstrap").
		Database("bootstrap").
		BinariesPath(absOut).
		DataPath(tmpData).
		RuntimePath(tmpRuntime))

	log.Println("[bootstrap-pgbin] downloading + extracting (this is the only step that needs internet)...")
	if err := pg.Start(); err != nil {
		log.Fatalf("start (forces download/extract): %v", err)
	}
	if err := pg.Stop(); err != nil {
		log.Fatalf("stop: %v", err)
	}

	// embedded-postgres's own existence check (see cmd/engine/main.go's
	// postgresBinariesPath doc comment) looks for "bin/pg_ctl" with no
	// extension even on Windows, which the extracted archive never
	// produces — without this, every launch re-triggers a pointless
	// re-extract. A byte-identical copy under the extension-less name
	// satisfies that check; Go's os/exec resolves the real "pg_ctl.exe"
	// fine either way when actually running it.
	exe := filepath.Join(absOut, "bin", "pg_ctl.exe")
	bare := filepath.Join(absOut, "bin", "pg_ctl")
	data, err := os.ReadFile(exe)
	if err != nil {
		log.Fatalf("read %s: %v", exe, err)
	}
	if err := os.WriteFile(bare, data, 0o755); err != nil {
		log.Fatalf("write %s: %v", bare, err)
	}

	log.Println("[bootstrap-pgbin] done:", absOut)
}
