package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

func get(url string) (body []byte, err error) {
	res, err := http.Get(url)
	if err != nil {
		return
	}

	body, err = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode > 299 {
		err = fmt.Errorf("[%d] endpoint: %s, body: %s\n", res.StatusCode, url, body)
	}
	return
}

func configPath(directory ...string) (dir string, err error) {
	if dir, err = os.UserHomeDir(); err == nil {
		dir = filepath.Join(append([]string{dir, ".config", "lite-xl"}, directory...)...)
	}

	return
}

func remove(path string) (err error) {
	if _, err = os.Stat(path); err == nil {
		err = os.RemoveAll(path)
	}

	return err
}

func clone(repoEndpoint, path string) (string, error) {
	var repo, name, commit, err = extract(repoEndpoint)
	if err != nil {
		return "", err
	}

	if path == "" {
		path, err = os.MkdirTemp("", name)
		if err != nil {
			return "", err
		}
	}

	cmd := exec.Command("git", "clone", repo, path)
	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("Cannot clone repository %s: %w", repo, err)
	}

	if commit != "" {
		cmd = exec.Command("git", "--git-dir="+filepath.Join(path, ".git"), "--work-tree="+path, "checkout", commit)
		if err = cmd.Run(); err != nil {
			return "", fmt.Errorf("Cannot checkout at %s: %w", commit, err)
		}
	}

	return path, nil
}

func extract(rawrepo string) (repo, name, commit string, err error) {
	rgx := regexp.MustCompile(`^(https?://[\w\-/\.]+/([\w\-\.]+)):?(\w+)?$`)
	res := rgx.FindStringSubmatch(rawrepo)
	switch len(res) {
	case 4:
		if cmt := res[3]; cmt != "last" && cmt != "latest" {
			commit = cmt
		}
		fallthrough
	case 3:
		repo = res[1]
		switch path.Ext(res[2]) {
		case "":
			name = res[2] + ".git"
		case ".git":
			name = res[2]
		default:
			err = fmt.Errorf("Unsupported extention")
		}
	default:
		err = fmt.Errorf("Malformed repository link: %s", rawrepo)
	}

	return
}

func truncAfter(s string, n int) string {
	if len(s) > n {
		return s[:n-3] + "..."
	}
	return s
}

type osEntry interface {
	IsDir() bool
	Name() string
}

func isRelevant(entry osEntry) bool {
	name := strings.ToLower(entry.Name())
	switch name {
	case "readme", "readme.md", "license", "license.md", "manifest.json":
		return false
	case ".git":
		return !entry.IsDir()
	}

	return !strings.HasPrefix(name, "test")
}

func moveDir(from, to string, perm os.FileMode) error {
	var e, queue = make(chan error, 1), make(chan string)

	from, to = filepath.Clean(from), filepath.Clean(to)

	go func() {
		defer close(queue)

		if err := os.MkdirAll(to, perm); err != nil {
			e <- err
			return
		}

		e <- filepath.WalkDir(from, func(path string, d os.DirEntry, errin error) (err error) {
			if errin != nil {
				return errin
			}

			if !d.IsDir() {
				queue <- path
			} else if path != from {
				err = os.Mkdir(filepath.Join(to, strings.TrimPrefix(path, from)), perm)
			}
			return
		})
	}()

	for path := range queue {
		tail := strings.TrimPrefix(path, from)
		if err := os.Rename(path, filepath.Join(to, tail)); err != nil {
			return err
		}
	}

	return <-e
}

func moveDirFiltered(from, to string, perm os.FileMode, allow func(string, os.DirEntry) bool) error {
	var e, queue = make(chan error, 1), make(chan string)

	from, to = filepath.Clean(from), filepath.Clean(to)

	go func() {
		defer close(queue)

		if err := os.MkdirAll(to, perm); err != nil {
			e <- err
			return
		}

		e <- filepath.WalkDir(from, func(path string, d os.DirEntry, errin error) (err error) {
			if errin != nil {
				return errin
			}

			if allow(path, d) {
				if !d.IsDir() {
					queue <- path
				} else if path != from {
					err = os.Mkdir(filepath.Join(to, strings.TrimPrefix(path, from)), perm)
				}
			} else if d.IsDir() {
				err = filepath.SkipDir
			}

			return
		})

	}()

	for path := range queue {
		tail := strings.TrimPrefix(path, from)
		if err := os.Rename(path, filepath.Join(to, tail)); err != nil {
			return err
		}
	}

	return <-e
}

func rangeSaved(each func(addon) error) error {
	ch, errch := make(chan []addon, len(aTypes)-1), make(chan error, 1)

	fn := func(t addonsType) {
		var list []addon
		from, err := configPath(t.folder())
		if err != nil {
			errch <- err
			return
		}

		err = filepath.WalkDir(from, func(path string, d os.DirEntry, errin error) (err error) {
			if errin != nil {
				return errin
			}

			name := d.Name()
			if path == from {
				return
			}

			if d.IsDir() {
				err = filepath.SkipDir
			}

			list = append(list, addon{
				ID:         name[:len(name)-len(filepath.Ext(name))],
				AddonsType: t,
				Path:       path,
			})
			return
		})
		if err != nil {
			errch <- err
		} else {
			ch <- list
		}
	}

	for i := range aTypes[:len(aTypes)-1] {
		go fn(addonsType(i))
	}

	for i := 0; i < len(aTypes)-1; {
		select {
		case addons := <-ch:
			for _, a := range addons {
				if err := each(a); err != nil {
					return err
				}
			}
			i++
		case err := <-errch:
			return err
		}
	}
	return nil
}
