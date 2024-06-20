package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const BASE_ENDPOINT = "https://raw.githubusercontent.com/lite-xl/lite-xl-plugins/master/"

type manifest struct {
	Addons  []addon     `json:"addons,omitempty"`
	Remotes []string    `json:"remotes,omitempty"`
	LiteXLs []lxlclient `json:"lite-xls,omitempty"`
}

var cache *manifest

func fetchManifest() (*manifest, error) {
	if cache == nil {
		cache = new(manifest)
		if raw, e := get(BASE_ENDPOINT + "manifest.json"); e != nil {
			return nil, fmt.Errorf("Error while retrieveing manifest: %s", e)
		} else if e = json.Unmarshal(raw, cache); e != nil {
			return cache, fmt.Errorf("Error while parsing manifest: %s", e)
		}
	}

	return cache, nil
}

type lxlclient struct {
	Version    string `json:"version,omitempty"`
	ModVersion any    `json:"mod_version,omitempty"`
	Files      []file `json:"files,omitempty"`
}

type post string

func (p *post) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &p); err == nil {
		return nil
	}

	m := make(map[string]string)
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}

	if s, ok := m[runtime.GOOS]; ok {
		*p = post(s)
		return nil
	}

	for key := range m {
		if strings.HasSuffix(key, runtime.GOOS) {
			*p = post(m[key])
			return nil
		}
	}
	return fmt.Errorf("Invalid post on: %s", b)
}

func (p post) execute() error {
	if p == "" {
		return nil
	}

	cmd := exec.Command(string(p))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	return cmd.Run()
}

type dependency struct {
	Version  string `json:"version,omitempty"`
	Optional bool   `json:"optional,omitempty"`
}

type file struct {
	Url      string `json:"url"`
	Checksum string `json:"checksum"`
	Arch     string `json:"arch"`
	Path     string `json:"path,omitempty"`
	Optional bool   `json:"optional,omitempty"`
}

var wrongOs error = fmt.Errorf("Mismached os")

func (f file) download() error {
	if f.Arch != "*" && !strings.HasSuffix(f.Arch, runtime.GOOS) {
		return wrongOs
	}

	content, err := get(f.Url)
	if err != nil {
		return err
	}

	local := f.Path
	if local == "" {
		local = path.Base(f.Url)
	}

	err = os.WriteFile(local, content, 0666)
	return err
}

type addonsType string

const (
	meta    addonsType = "meta"
	font    addonsType = "font"
	library addonsType = "library"
	plugin  addonsType = "plugin"
	color   addonsType = "color"
)

func (t addonsType) String() string {
	if t == "" {
		return string(plugin)
	}
	return string(t)
}

func (t *addonsType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	switch addonsType(s) {
	case "":
		*t = plugin
	case font, library, plugin, color, meta:
		*t = addonsType(s)
	default:
		return fmt.Errorf("Unrecognized addon type: %s", s)
	}

	return nil
}

func (t addonsType) folder() string {
	switch t {
	case color, font, plugin:
		return string(t) + "s"
	case library:
		return "libraries"
	}
	return plugin.folder()
}

type addon struct {
	ID           string                 `json:"id"`
	Version      string                 `json:"version"`
	ModVersion   any                    `json:"mod_version,omitempty"`
	AddonsType   addonsType             `json:"type"`
	Name         string                 `json:"name,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Provides     []string               `json:"provides,omitempty"`
	Replaces     []string               `json:"replaces,omitempty"`
	Remote       string                 `json:"remote,omitempty"`
	Dependencies map[string]*dependency `json:"dependencies,omitempty"`
	Conflicts    map[string]*dependency `json:"conflicts,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	Path         string                 `json:"path,omitempty"`
	Arch         []string               `json:"arch,omitempty"`
	Post         post                   `json:"post,omitempty"`
	Url          string                 `json:"url,omitempty"`
	Checksum     string                 `json:"checksum,omitempty"`
	Extra        map[string]string      `json:"extra,omitempty"`
	Files        []file                 `json:"files,omitempty"`
}

func (a addon) dir(subdir ...string) (string, error) {
	var path = a.Path
	if path == "" && len(a.Files) == 1 && a.Files[0].Path != "" {
		path = a.Files[0].Path
	}

	switch path {
	case "", ".":
		path = filepath.Join(a.AddonsType.folder(), a.ID)
	}

	return configPath(append([]string{path}, subdir...)...)
}

func (a addon) endpoint() (endpoint string, singleton bool, err error) {
	if a.Url != "" {
		endpoint = a.Url
	} else if a.Remote == "" {
		switch len(a.Files) {
		case 0:
			endpoint = BASE_ENDPOINT + a.AddonsType.folder() + "/" + a.ID + ".lua"
		case 1:
			endpoint = a.Files[0].Url
		default:
			err = fmt.Errorf("Cannot find valid endpoint")
		}
	} else if strings.HasPrefix(a.Remote, "http") {
		endpoint = a.Remote
	} else {
		endpoint = BASE_ENDPOINT + a.Remote
	}

	singleton = strings.HasSuffix(endpoint, ".lua")

	return
}

func (a addon) supported() (supported bool) {
	if len(a.Arch) == 0 {
		return true
	}

	for _, arch := range a.Arch {
		if arch == "*" || strings.HasSuffix(arch, runtime.GOOS) {
			supported = true
			return
		}
	}
	return
}
