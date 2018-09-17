package hab

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// PackagesInfo is response from depotj
type PackagesInfo struct {
	RangeStart  int           `json:"range_start"`
	RangeEnd    int           `json:"range_end"`
	TotalCount  int           `json:"total_count"`
	PackageList []PackageInfo `json:"data"`
}

// PackageInfo is package info in pkgs response
type PackageInfo struct {
	Origin   string   `json:"origin"`
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Release  string   `json:"release"`
	Channels []string `json:"channels"`
}

// Depot for hab
type Depot interface {
	PackageVersionsFromName(pkgName string, habChannel string) ([]string, error)
}

type depot struct {
	baseURL string
	client  *http.Client
}

// New returns a new depot object
func New(baseURL string) Depot {
	return &depot{baseURL, &http.Client{Timeout: 10 * time.Second}}
}

// packagesInfo fetch packages info from depot
func (depo *depot) packagesInfo(pkgName string, from int) (PackagesInfo, error) {
	pkgURL := fmt.Sprintf("%s/pkgs/%s?range=%d", depo.baseURL, pkgName, from)
	res, err := depo.client.Get(pkgURL)

	if err != nil {
		return PackagesInfo{}, err
	}

	defer res.Body.Close()

	if res.StatusCode == 404 {
		return PackagesInfo{}, errors.New("Package not found")
	} else if res.StatusCode/100 != 2 {
		return PackagesInfo{}, fmt.Errorf("Unexpected status code: %d", res.StatusCode)
	}

	var pkgsInfo PackagesInfo
	err = json.NewDecoder(res.Body).Decode(&pkgsInfo)

	if err != nil {
		return PackagesInfo{}, err
	}

	return pkgsInfo, nil
}

// PackageVersionsFromName fetch all versions from depot
func (depo *depot) PackageVersionsFromName(pkgName string, habChannel string) ([]string, error) {
	var packages []PackageInfo

	offset := 0
	for {
		pkgsInfo, err := depo.packagesInfo(pkgName, offset)

		if err != nil {
			return nil, err
		}

		packages = append(packages, pkgsInfo.PackageList...)

		offset = pkgsInfo.RangeEnd + 1

		if offset >= pkgsInfo.TotalCount {
			break
		}
	}

	var versions []string
	foundVersions := map[string]bool{}
	for _, pkg := range packages {
		if foundVersions[pkg.Version] {
			continue
		}
		for _, channel := range pkg.Channels {
			if channel == habChannel {
				versions = append(versions, pkg.Version)
				foundVersions[pkg.Version] = true
				break
			}
		}
	}

	return versions, nil
}
