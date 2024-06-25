# lxl
A WIP plugin manager for [lite-xl](https://github.com/lite-xl/lite-xl) text editor

## Build
As easy as `go build`, same in all OSs

## Usage
_Manage your addons_
 - **find** any addons from the updated list `lxl find <addon>` (addon argument is optional)
 - **install** a specific addon `lxl install <addonID>`
 - **uninstall** a specific addon `lxl uninstall <addonID>`
 - **list** all installed addons `lxl list <addon>` (addon argument is optional)

_Manage your remotes_
> A remote is a link of a [manifest.json](https://github.com/adamharrison/lite-xl-plugin-manager/blob/master/SPEC.md) that contains might contains new addons to discover.
> By default official ones
> ([plugins](https://github.com/lite-xl/lite-xl-plugins/blob/master/manifest.json),
[colors](https://raw.githubusercontent.com/lite-xl/lite-xl-colors/master/manifest.json),
[lsp-servers](https://github.com/lite-xl/lite-xl-lsp-servers/blob/main/manifest.json) and
[ide](https://github.com/lite-xl/lite-xl-ide/blob/main/manifest.json)) are already supported
 - **subscribe** to a specific remote `lxl subscribe <remote>`
 - **unsubscribe** from a specific remote `lxl unsubscribe <evaluated-remote>`
 - list **remotes** that lxl is subscribed `lxl remotes`

## To do
- Support stud
- Proper versioning management
- Verbose and plumbing mode
- Filter by
- GUI via lite-xl plugin
