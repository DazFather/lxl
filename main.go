package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

func main() {
	var err error
	onExit := func() {
		if err == nil {
			success(os.Args[1], "Completed successfully")
		} else if err != skip {
			danger("Unable to "+os.Args[1], err)
		}
	}
	defer onExit()

	switch len(os.Args) {
	case 2:
		switch os.Args[1] {
		case "help":
			success(os.Args[1], USAGE)
			err = skip
			return
		case "list":
			err = list("")
			return
		case "find":
			err = find("")
			return
		case "remotes":
			err = remotes("")
			return
		}
		fallthrough
	case 0, 1:
		warn("Invalid arguments", USAGE)
		err = skip
		return
	}

	switch os.Args[1] {
	case "list":
		err = list(os.Args[2])
	case "install":
		err = install(os.Args[2])
	case "uninstall":
		err = uninstall(os.Args[2])
	case "find":
		err = find(os.Args[2])
	case "subscribe":
		err = subscribe(os.Args[2])
	case "unsubscribe":
		err = unsubscribe(os.Args[2])
	case "remotes":
		err = remotes(os.Args[2])
	default:
		danger("Unrecognized command", USAGE)
		err = skip
		return
	}
}

func find(addonID string) (err error) {
	// Retrieve manifest
	manifest, err := fetchManifest()
	if err != nil {
		return
	}

	// Finding addon
	var found []addon
	if addonID == "" {
		found = manifest.Addons
	} else {
		addonID = strings.ToLower(addonID)
		for _, item := range manifest.Addons {
			if strings.Contains(strings.ToLower(item.ID), addonID) {
				found = append(found, item)
			}
		}
	}

	if err = showAddons(os.Args[1], found); addonID == "" && err == nil {
		success(os.Args[1]+" tip", "Use ", os.Args[1], " followed by something to filter results")
	}
	return
}

func uninstall(addonID string) (err error) {
	var delted bool

	err = rangeSaved(func(a addon) (e error) {
		if a.ID == addonID {
			if e = remove(a.Path); e == nil {
				delted = true
			}
		}
		return
	})

	if err == nil && !delted {
		err = fmt.Errorf("Cannot find \"%s\" addon", addonID)
	}

	return
}

func install(addonID string) (err error) {

	// Retrieve manifest
	manifest, err := fetchManifest()
	if err != nil {
		return
	}

	// Finding addon
	var found = cache.retrieve(addonID)
	fmt.Println("installing", addonID, "from:", found.repo)
	if found == nil {
		return fmt.Errorf("Cannot find %s addon", addonID)
	} else if installed, e := found.isInstalled(); installed {
		return fmt.Errorf("%s already installed", addonID)
	} else if e != nil {
		return e
	} else if !found.supported() {
		return fmt.Errorf("Addon does not support your OS")
	}

	local, err := found.dir()
	if err != nil {
		return
	}
	fmt.Println("LOCAL", local)

	repo, singleton, err := found.endpoint()
	if err != nil {
		return
	}

	fmt.Println("ENPOINT", repo)

	if singleton {
		content := []byte{}
		if strings.HasPrefix(repo, "http") {
			content, err = get(repo)
		} else {
			content, err = os.ReadFile(repo)
		}
		if err == nil {
			err = os.WriteFile(local, content, 0666)
		}
	} else {
		switch found.Path {
		case ".", filepath.Join(found.AddonsType.folder(), found.ID):
			if _, err = clone(repo, local); err != nil {
				return
			}
		default:
			err = found.installRepo(repo, local)
		}
	}

	if err != nil {
		return
	}

	// Installing addon

	// Removing old dependencies
	for _, dep := range found.Replaces {
		err = uninstall(dep)
		if err != nil {
			return err
		}
	}

	for _, item := range manifest.Addons {
		// Check for conflicts
		for dep := range found.Conflicts {
			if item.ID != dep {
				continue
			}

			if path, e := item.dir(); e != nil {
				return e
			} else if e := remove(path); err != nil && !os.IsNotExist(e) {
				return e
			}
		}

		// Installing dependencies
		for dep := range found.Dependencies {
			if item.ID != dep {
				continue
			}

			isInstalled := false
			isInstalled, err = item.isInstalled()
			if isInstalled {
				continue
			} else if err != nil {
				return
			}

			if err = install(dep); err != nil && !found.Dependencies[dep].Optional {
				err = fmt.Errorf("Cannot install mandatory dependency %s: %s", dep, err)
				return
			}
		}
	}

	return
}

func list(addonID string) (err error) {
	var list []addon
	addonID = strings.ToLower(addonID)

	err = rangeSaved(func(a addon) error {
		if strings.Contains(a.ID, addonID) {
			list = append(list, a)
		}
		return nil
	})
	if err != nil {
		return
	}

	// Retrieve manifest
	manifest, err := fetchManifest()
	if err != nil {
		return
	}

	for _, item := range manifest.Addons {
		if item.AddonsType == meta {
			continue
		}

		for i := range list {
			if list[i].ID == item.ID {
				list[i] = item
			}
		}
	}

	return showAddons(os.Args[1], list)
}

func subscribe(repo string) error {
	return updateStatus(func(l *lxl) error {
		added, e := l.addRemote(repo)
		if e == nil && !added {
			e = fmt.Errorf("evaluated remote %s is already present on lxl remote list", repo)
		}
		return e
	})
}

func unsubscribe(repo string) error {
	return updateStatus(func(l *lxl) error {
		if ind := slices.Index(l.Remotes, repo); ind > 0 {
			l.Remotes = append(l.Remotes[:ind], l.Remotes[ind+1:]...)
			return nil
		}
		return fmt.Errorf("cannot find %s remote", repo)
	})
}

func remotes(filter string) (err error) {
	if err = loadStatus(); err != nil {
		return
	}

	switch size := len(cache.Remotes); size {
	case 0:
		err = fmt.Errorf("No remote found")
	case 1:
		success("Found one remote", "details:")
		fmt.Println(showRemote(cache.Remotes[0]))
	default:
		success("Found "+strconv.Itoa(size)+" remotes", "List of avaiable remotes:")

		remoteCh := make(chan string, size)
		for _, u := range cache.Remotes {
			go func(url string) {
				remoteCh <- showRemote(url)
			}(u)
		}

		i := 0
		for screen := range remoteCh {
			fmt.Println(screen)
			if i++; i == size {
				close(remoteCh)
			}
		}
	}

	fmt.Print("\n\nYou can manage your remote using the following command:\n add a new remote ")
	command(" lxl subscribe <remote> ")
	fmt.Print(" remove a remote  ")
	command(" lxl unsubscribe <remote> ")
	fmt.Println()
	warn("remote will be evaluated", "When adding a new remote the link will be evaluated for performance optimization, therefore original will be lost and then new one will appear on the list\n")

	return
}
