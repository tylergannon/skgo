package newapp

import (
	"github.com/tylergannon/skgo/internal/adapter"
	"golang.org/x/mod/semver"
	"net/http"
	"time"
)

// CompatiblePackages uses the same sparse-publication selection as creation.
// Update persists these exact versions rather than a moving npm range.
func CompatiblePackages(version string) (adapterVersion, addonVersion string, err error) {
	client := &http.Client{Timeout: 30 * time.Second}
	compatible := func(v string) bool { return semver.Compare("v"+v, version) <= 0 }
	adapterVersion, err = highestVersion(client, defaultRegistry, adapter.Package, compatible)
	if err != nil {
		return
	}
	addonVersion, err = highestVersion(client, defaultRegistry, svAddonPackage, compatible)
	return
}
