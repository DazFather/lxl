package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var err error

	switch len(os.Args) {
	case 2:
		switch os.Args[1] {
		case "help":
			success(os.Args[1], USAGE)
			return
		case "list":
			if err = list(""); err != nil {
				danger("Unable to "+os.Args[1], err)
			}
			return
		case "find":
			success(os.Args[1], "Use ", os.Args[1], " followed by something to filter results")
			if err = find(""); err != nil {
				danger("Unable to "+os.Args[1], err)
			}
			return
		}
		fallthrough
	case 0, 1:
		warn("Invalid arguments", USAGE)
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
	default:
		danger("Unrecognized command", USAGE)
		return
	}

	if err != nil {
		danger("Unable to "+os.Args[1], err)
	} else {
		success(os.Args[1]+" \""+os.Args[2]+"\"", "Completed successfully")
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

	return showAddons(os.Args[1], found)
}

func uninstall(addonID string) (err error) {
	var filepath string
	var delted bool

	addonID = strings.TrimSuffix(addonID, ".lua")

	for _, atype := range []addonsType{plugin, font, library, color} {
		if filepath, err = configPath(atype.folder(), addonID); err != nil {
			return
		}

		if err = remove(filepath); err == nil {
			delted = true
		} else if os.IsNotExist(err) {
			if err = remove(filepath + ".lua"); err == nil {
				delted = true
			}
		} else {
			return
		}
	}

	if !delted {
		err = fmt.Errorf("Cannot find \"%s\" addon", addonID)
	}
	if os.IsNotExist(err) {
		err = nil
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
	// Retrieve manifest
	manifest, err := fetchManifest()
	if err != nil {
		return
	}

	addonID = strings.ToLower(addonID)

	var (
		i         int
		list      []addon
		foundEach = make(chan []addon, len(aTypes)-1)
		errch     = make(chan error, 1)
	)
	for i := range aTypes[:len(aTypes)-1] {
		go func(t addonsType) {
			list, err := collectDir(t, addonID)
			if err != nil {
				errch <- err
			}
			foundEach <- list
		}(addonsType(i))
	}

out:
	for {
		select {
		case addons := <-foundEach:
			list = append(list, addons...)
			if i++; len(aTypes)-1 == i {
				close(errch)
				break out
			}
		case err = <-errch:
			return err
		}
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
					fmt.Println("WARING: stud currently not supported")
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

		err = moveDirFiltered(path, local, 0750, func(_ string, d os.DirEntry) bool {
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
