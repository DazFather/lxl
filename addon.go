package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

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
	Arch     arch   `json:"arch,omitempty"`
	Path     string `json:"path,omitempty"`
	Optional bool   `json:"optional,omitempty"`
}

type arch []string

func (a *arch) UnmarshalJSON(b []byte) (err error) {
	switch b[0] {
	case '"':
		list := make([]string, 1)
		if err = json.Unmarshal(b, &list[0]); err == nil {
			*a = arch(list)
		}
	case '[':
		if len(b) == 2 && b[1] == ']' {
			return
		}

		list := make([]string, 0)
		if err = json.Unmarshal(b, &list); err == nil {
			*a = arch(list)
		}
	default:
		err = fmt.Errorf("Invalid arch: %s", b)
	}

	return
}

func (a arch) supported() bool {
	switch len(a) {
	case 0:
		return true
	case 1:
		return a[0] == "" || a[0] == "*" || strings.HasSuffix(a[0], runtime.GOOS)
	}

	for i := range a {
		if strings.HasSuffix(a[i], runtime.GOOS) {
			return true
		}
	}
	return false
}

var wrongOs error = fmt.Errorf("Mismached os")

func (f file) download() error {
	if !f.Arch.supported() {
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

type addonsType uint8

const (
	plugin addonsType = iota
	font
	library
	color
	meta
)

var aTypes = []string{"plugin", "font", "library", "color", "meta"}

func (t addonsType) String() string {
	return aTypes[t]
}

func (t *addonsType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	for i := range aTypes {
		if s == aTypes[i] {
			*t = addonsType(i)
		}
	}

	if t == nil {
		return fmt.Errorf("Unrecognized addon type: %s", s)
	}
	return nil
}

func (t addonsType) folder() string {
	switch t {
	case color, font, plugin:
		return t.String() + "s"
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
	repo         string
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
			endpoint, err = url.JoinPath(a.repo, a.AddonsType.folder(), a.ID+".lua")
		case 1:
			endpoint = a.Files[0].Url
		default:
			err = fmt.Errorf("Cannot find valid endpoint")
		}
	} else if strings.HasPrefix(a.Remote, "http") {
		endpoint = a.Remote
	} else {
		endpoint, err = url.JoinPath(a.repo, a.Remote)
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

func (a addon) install() error {
	if !a.supported() {
		return fmt.Errorf("plugin does not support your OS")
	}

	var repo, singleton, err = a.endpoint()
	if err != nil {
		return err
	}

	local, err := a.dir()
	if err != nil {
		return err
	}

	if singleton {
		if !strings.HasSuffix(local, ".lua") {
			local += ".lua"
		}

		content, err := get(repo)
		if err == nil {
			err = os.WriteFile(local, content, 0666)
		}
		return err
	}

	switch a.Path {
	case ".", filepath.Join(a.AddonsType.folder(), a.ID):
		_, err = clone(repo, local)
	default:
		path, e := clone(repo, "")
		if e != nil {
			return e
		}
		defer remove(path)

		// Detecting singleton
		entries, e := os.ReadDir(path)
		if e != nil {
			return e
		}
		var init *string
		for _, item := range entries {
			if !isRelevant(item) {
				if item.Name() == "manifest.json" {
					warn("stud detected", "studs are not currently supported")
				}
				continue
			}

			if init == nil {
				init = new(string)
				*init = item.Name()
			} else {
				init = nil
				break
			}
		}
		// Singleton detected
		if init != nil {
			local += ".lua"
			err = os.Rename(filepath.Join(path, *init), local)
			break
		}

		err = moveDirFiltered(path, local, func(_ string, d os.DirEntry) bool {
			return isRelevant(d)
		})

		if err != nil {
			remove(local)
		}
	}

	if err != nil {
		return err
	}

	for _, f := range a.Files {
		if err = f.download(); err != nil && err != wrongOs && !f.Optional {
			return err
		}
	}

	return a.Post.execute()
}
