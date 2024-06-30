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
	ModVersion   string                 `json:"mod_version,omitempty"`
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

	if path == "" && len(a.Files) == 1 {
		if fpath := a.Files[0].Path; fpath != "" {
			path = fpath
		} else {
			path = filepath.Base(a.Files[0].Url)
		}
	}

	switch path {
	case ".":
		path = a.ID
	case "":
		return "", nil
	}

	return configPath(append([]string{a.AddonsType.folder(), path}, subdir...)...)
}

func buildEndpoint(a addon) (endpoint string, err error) {
	var u *url.URL

	if !strings.HasPrefix(a.repo, "http") {
		endpoint = filepath.Join(a.repo, a.Path)
	} else if u, err = url.Parse(a.repo); err == nil {
		u.Path = path.Dir(u.Path)
		endpoint, err = url.JoinPath(u.String(), a.AddonsType.folder(), a.ID+".lua")
	}

	return
}

func (a addon) endpoint() (endpoint string, singleton bool, err error) {
	if a.Url != "" {
		endpoint = a.Url
	} else if a.Remote == "" {
		switch len(a.Files) {
		case 0:
			singleton = true
			endpoint, err = buildEndpoint(a)
		case 1:
			singleton = true
			endpoint = a.Files[0].Url
		default:
			err = fmt.Errorf("Cannot find valid endpoint")
		}
	} else if strings.HasPrefix(a.Remote, "http") {
		endpoint = a.Remote
	} else {
		singleton = true
		endpoint, err = buildEndpoint(a)
	}

	if !singleton {
		singleton = strings.HasSuffix(endpoint, ".lua")
	}

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

func (a addon) isInstalled() (installed bool, err error) {
	var path string

	if path, err = a.dir(); err != nil {
		return
	}

	if _, err = os.Stat(path); err == nil {
		installed = true
	} else if os.IsNotExist(err) {
		if _, err = os.Stat(path + ".lua"); err == nil {
			installed = true
		} else if os.IsNotExist(err) {
			err = nil
		}
	}

	return
}

func stub(self *addon, manifestPath string) (foundself bool, err error) {
	var stub *manifest
	if stub, err = fetchLocalManifest(manifestPath); err != nil {
		err = fmt.Errorf("Cannot correctly retrieve stub: %s", err)
		return
	}

	var toInstall = make([]*addon, len(stub.Addons))
	for i, stubAdd := range stub.Addons {
		fmt.Println("-", stubAdd.ID, stubAdd.Version)
		if stubAdd.ID != self.ID {
			//if found := cache.retrieve(self.ID); found != nil {
			//	fmt.Println(" copying repo (ID", self.ID, ")", found.repo)
			//}
			fmt.Println("\t- ADDING", stubAdd.ID, "vs", self.ID)
			// stubAdd.repo = self.repo
			//tmp := stubAdd
			toInstall[i] = new(addon)
			*toInstall[i] = stubAdd
		} else if stubAdd.Version != self.Version || stubAdd.ModVersion != self.ModVersion || stubAdd.Remote != self.Remote {

			warn("broken stub", "version on stub diverge from repository, installation may be inconsistent")
			// if found := cache.retrieve(self.ID); found != nil {
			// 	fmt.Println(" copying repo (V.", self.Version, ")", found.repo)
			// 	stubAdd.repo = found.repo
			// }
			tmp := stubAdd
			cache.installing = append(cache.installing, &tmp)
			return true, install(stubAdd.ID)
		}
	}

	errch := make(chan error, len(toInstall))
	defer close(errch)
	for _, add := range toInstall {
		if add == nil {
			errch <- nil
			continue
		}
		cache.installing = append(cache.installing, add)
		go func(addonID string) {
			errch <- install(addonID)
		}(add.ID)
	}

	i := 0
	for e := range errch {
		if e != nil {
			warn("error douring stub", e)
			err = e
		}
		if i++; i == len(toInstall) {
			break
		}
	}

	if err != nil {
		for _, add := range toInstall {
			if add != nil {
				uninstall(add.ID)
			}
		}
		err = fmt.Errorf("Cannot install all stub dependencies, previous state has been repristinated but there still might be some inconsistencies")
	}

	return
}

func (a *addon) installRepo(repo, local string) (err error) {
	var path string

	fmt.Println("cloning", repo)
	if path, err = clone(repo, ""); err != nil {
		return
	}
	fmt.Println("END cloning")
	defer remove(path)

	// Detecting singleton & stub
	entries, err := os.ReadDir(path)
	if err != nil {
		return
	}
	var init *string
	for _, item := range entries {
		name := item.Name()
		if isRelevant(item) {
			if init == nil {
				init = new(string)
				*init = name
			} else {
				*init = ""
			}
			continue
		}

		// stub detected
		if name == "manifest.json" {
			fmt.Println("stub detected", a.repo)
			self := false
			if self, err = stub(a, filepath.Join(path, name)); self || err != nil {
				return
			}
		}
	}
	// Singleton detected
	if init != nil && *init != "" {
		if filepath.Ext(local) == "" {
			local += filepath.Ext(*init)
		}
		err = os.Rename(filepath.Join(path, *init), local)
	} else if local != "" {
		err = moveDirFiltered(path, local, func(_ string, d os.DirEntry) bool { return isRelevant(d) })
	} else {
		warn("I dunno what to do")
	}

	if err == nil {
		err = a.Post.execute()
	}

	return
}
