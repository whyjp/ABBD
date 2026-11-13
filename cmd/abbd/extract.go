package main

import (
	"os"
	"path/filepath"

	"github.com/whyjp/abbd/internal/extract/frontend"
)

// extractBundled prints the extraction JSON the bundled front-ends read
// from the build systems' own models. It reads only.
func (e env) extractBundled(args []string) error {
	var specs []frontend.Spec
	for _, a := range args {
		s, err := frontend.ParseSpec(a)
		if err != nil {
			return err
		}
		specs = append(specs, s)
	}
	doc, err := frontend.ExtractWith(e.root, cacheDir(), specs, e.cacheOpts)
	if err != nil {
		return err
	}
	return doc.Encode(e.stdout)
}

// cacheDir holds the build trees a front-end configures, outside every
// repository: $ABBD_CACHE_DIR, else the user cache directory.
func cacheDir() string {
	if d := os.Getenv("ABBD_CACHE_DIR"); d != "" {
		return d
	}
	d, err := os.UserCacheDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "abbd")
}
