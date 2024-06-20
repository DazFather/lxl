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
		fmt.Errorf("Response failed with status code: %d, body: %s\n", res.StatusCode, body)
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

	commands := [][]string{
		{"clone", repo, path},
		{"--git-dir=" + filepath.Join(path, ".git"), "--work-tree=" + path, "checkout", commit},
	}

	for _, args := range commands {
		cmd := exec.Command("git", args...)
		if err = cmd.Run(); err != nil {
			return "", fmt.Errorf("Cannot execute:\n git %s\n%w", strings.Join(args, " "), err)
		}
	}

	return path, nil
}

func extract(rawrepo string) (repo, folder, commit string, err error) {
	rgx := regexp.MustCompile(`^(https?://[\w\-/\.]+/([\w+\-]+)\.?g?i?t?):(\w+)$`)
	res := rgx.FindStringSubmatch(rawrepo)
	if len(res) != 4 {
		err = fmt.Errorf("Malformed repository link: %s", rawrepo)
		return
	}

	if path.Ext(res[1]) == "" {
		res[1] += ".git"
	}

	repo, folder, commit = res[1], res[2], res[3]
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

func moveDirFiltered(from, to string, perm os.FileMode, allow func(string, os.DirEntry) bool) error {
	from, to = filepath.Clean(from), filepath.Clean(to)

	type msg struct {
		name    string
		content []byte
	}
	e := make(chan error, 1)
	queue := make(chan msg)

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

			if d.IsDir() {
				if path == from {
					return nil
				}
				if !allow(path, d) {
					return filepath.SkipDir
				}
				return os.Mkdir(filepath.Join(to, strings.TrimPrefix(path, from)), perm)
			} else if !allow(path, d) {
				return nil
			}

			content, err := os.ReadFile(path)
			if err == nil {
				queue <- msg{strings.TrimPrefix(path, from), content}
			}

			return
		})

	}()

	for item := range queue {
		f, err := os.Create(filepath.Join(to, item.name))
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = f.Write(item.content)
		if err != nil {
			return err
		}
	}

	return <-e
}

func moveDir(from, to string, perm os.FileMode) error {
	from, to = filepath.Clean(from), filepath.Clean(to)

	type msg struct {
		name    string
		content []byte
	}
	e := make(chan error, 1)
	queue := make(chan msg)

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

			if d.IsDir() {
				if path == from {
					return nil
				}
				return os.Mkdir(filepath.Join(to, strings.TrimPrefix(path, from)), perm)
			}

			content, err := os.ReadFile(path)
			if err == nil {
				queue <- msg{strings.TrimPrefix(path, from), content}
			}

			return
		})

	}()

	for item := range queue {
		f, err := os.Create(filepath.Join(to, item.name))
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = f.Write(item.content)
		if err != nil {
			return err
		}
	}

	return <-e
}
