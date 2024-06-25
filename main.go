package main

import (
	"fmt"
	"os"
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
	var found *addon
	for _, item := range manifest.Addons {
		if item.ID == addonID {
			found = &item
			break
		}
	}
	if found == nil {
		return fmt.Errorf("Cannot find %s addon", addonID)
	}

	// Check for conflicts
	for _, item := range manifest.Addons {
		for dep := range found.Conflicts {
			if item.ID != dep {
				continue
			}

			if path, e := item.dir(); e != nil {
				return e
			} else if e := remove(path); !os.IsNotExist(e) {
				return e
			}
		}
	}

	// Removing old dependencies
	for _, dep := range found.Replaces {
		err = uninstall(dep)
		if err != nil {
			return err
		}
	}

	// Installing dependencies
	for _, item := range manifest.Addons {
		for dep := range found.Dependencies {
			if item.ID == dep {
				if err = item.install(); err != nil {
					return
				}
			}
		}
	}

	// Installing addon
	return found.install()
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
		added, e := l.add(repo)
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
