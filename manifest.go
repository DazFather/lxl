package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

const (
	GITHUB_RAW_HOST = "raw.githubusercontent.com"
	GITHUB_HOST     = "github.com"

	BASE_ENDPOINT = "https://" + GITHUB_RAW_HOST + "/lite-xl/"
)

type liteXlClient struct {
	Version    string `json:"version,omitempty"`
	ModVersion any    `json:"mod_version,omitempty"`
	Files      []file `json:"files,omitempty"`
}

type manifest struct {
	Addons  []addon        `json:"addons,omitempty"`
	Remotes []string       `json:"remotes,omitempty"`
	LiteXLs []liteXlClient `json:"lite-xls,omitempty"`
}

type lxl struct {
	Remotes []string
	Path    string
	*manifest
}

var cache *lxl

func (l *lxl) has(reference string) (bool, error) {
	var commit string
	if ind := strings.LastIndexByte(reference, ':'); ind > 0 {
		reference, commit = reference[:ind], reference[ind+1:]
		if commit != "latest" && commit != "last" {
			return false, fmt.Errorf("Unsupported commit specifier on remote")
		}
	}

	if u, err := url.Parse(reference); err == nil {
		reference = u.Path
	} else {
		return false, err
	}

	has := slices.ContainsFunc(l.Remotes, func(item string) bool {
		return strings.Contains(item, reference)
	})
	return has, nil
}

func (l *lxl) add(reference string) (bool, error) {
	var commit string
	if ind := strings.LastIndexByte(reference, ':'); ind > 0 {
		reference, commit = reference[:ind], reference[ind+1:]
		if commit != "latest" && commit != "last" {
			return false, fmt.Errorf("Unsupported commit specifier on remote")
		}
	}

	u, err := url.Parse(reference)
	if err != nil {
		return false, err
	}

	switch strings.ToLower(u.Host) {
	case GITHUB_HOST:
		u.Host = GITHUB_RAW_HOST
		fallthrough
	case GITHUB_RAW_HOST:
		reference = u.String()
	default:
		if raw, e := get(reference); e != nil {
			return false, fmt.Errorf("Cannot retrieve manifest: %s", e)
		} else if e = json.Unmarshal(raw, new(manifest)); e != nil {
			return false, fmt.Errorf("Error while parsing manifest: %s", e)
		}
	}

	has := slices.Contains(l.Remotes, reference)
	if !has {
		l.Remotes = append(l.Remotes, reference)
	}
	return !has, nil
}

func fetchManifestAt(endpoint string) (m *manifest, err error) {
	m = new(manifest)
	if raw, e := get(endpoint); e != nil {
		err = fmt.Errorf("Cannot retrieve manifest from %s: %s", endpoint, e)
	} else if err = json.Unmarshal(raw, m); err != nil {
		err = fmt.Errorf("Error while parsing manifest from %s: %s", endpoint, err)
	} else if len(m.Remotes) > 0 {
		newUrls := []string{}
		for _, r := range m.Remotes {
			if has, err := cache.has(r); err != nil && !has {
				newUrls = append(newUrls, r)
			}
		}

		header := ""
		switch len(newUrls) {
		case 0:
			for i := range m.Addons {
				m.Addons[i].repo = endpoint
			}
			return
		case 1:
			header = "Found one new remote"
		default:
			header += "Found new remotes"
		}

		success(header, "A remote might contains new addons that would be avaiable to your lxl to find and install. You can add remote via:")
		for _, u := range newUrls {
			fmt.Print("  ")
			command(" lxl subscribe " + u + " ")
		}
		fmt.Print("\n\n")
	}

	if m != nil {
		for i := range m.Addons {
			m.Addons[i].repo = endpoint
		}
	}

	return
}

func fetchManifest() (*manifest, error) {
	if cache != nil {
		return cache.manifest, nil
	} else if err := loadStatus(); err != nil {
		return nil, err
	}
	size := len(cache.Remotes)

	manifestCh := make(chan *manifest, size)
	errorCh := make(chan error, size)
	for _, u := range cache.Remotes {
		go func(url string) {
			m, err := fetchManifestAt(url)
			if err != nil {
				errorCh <- err
			}
			manifestCh <- m
		}(u)
	}

	e := 0
	for i := 0; i < size; {
		select {
		case err := <-errorCh:
			warn("Error with a remote", err)
			e++
		case m := <-manifestCh:
			i++
			if m == nil {
				continue
			}
			if cache.manifest == nil {
				cache.manifest = m
				continue
			}
			cache.manifest.Addons = append(cache.manifest.Addons, m.Addons...)
		}
	}
	if e == size {
		return nil, fmt.Errorf("No valid remote")
	}
	close(errorCh)

	cache.Addons = slices.CompactFunc(cache.Addons, func(a, b addon) bool {
		return a.ID == b.ID
	})
	return cache.manifest, nil
}
