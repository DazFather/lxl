package main

import (
	"fmt"
	"github.com/DazFather/brush"
	"strconv"
	"strings"
)

const USAGE = `Usage:
 lxl <install|uninstall|find|list> <pluginID>
 lxl <subscribe|unsubscribe|remotes> <remote>`

// Palette
var (
	warn    = newPrinter(brush.Yellow, " ! ")
	danger  = newPrinter(brush.Red, " X ")
	success = newPrinter(brush.Green, " v ")
	command = newPrinter(brush.Black, " $ ")
)

func newPrinter(baseTone brush.ANSIColor, prefix string) func(...any) {
	white := brush.New(brush.BrightWhite, brush.UseColor(baseTone))
	black := brush.New(brush.Black, brush.UseColor(baseTone+8))

	return func(v ...any) {
		var suffix string = "\n"
		if len(v) >= 2 {
			suffix = brush.Paintln(baseTone, nil, v[1:]...).String()
			v[0] = fmt.Sprint(" ", v[0], " ")
			v = v[0:1]
		}
		fmt.Printf("%s%s %s", white.Paint(prefix), black.Paint(v...), suffix)
	}
}

func (t addonsType) color() brush.ANSIColor {
	return brush.ANSIColor(t) + 9
}

func (t addonsType) icon() brush.Painted {
	s := t.String()
	ch := " " + strings.ToUpper(s[:1]) + " "

	return brush.Paint(brush.BrightWhite, brush.UseColor(t.color()), ch)
}

func (a addon) snippet(maxDesc int) brush.Highlighted {
	desc := a.Description
	if len(desc) > maxDesc {
		desc = desc[:maxDesc-3] + "..."
	}

	color := a.AddonsType.color()

	return brush.Join(
		a.AddonsType.icon(),
		brush.Paint(color, nil, " ", a.ID),
		"\t"+desc,
	)
}

func (a addon) showcase() {
	color := a.AddonsType.color()

	fmt.Print(brush.Join(
		brush.Paint(brush.White, brush.UseColor(color), " ", strings.ToUpper(a.AddonsType.String()), " "),
		brush.Paint(color, nil, "\t"+a.ID),
		"\tv. ", a.Version,
		"\nDescription: ", a.Description,
		"\n\nTo install latest version use command:\n ",
	))
	command("lxl install ", a.ID, " ")
}

func showAddons(header string, addons []addon) error {
	switch n := len(addons); n {
	case 0:
		return fmt.Errorf("Cannot find any addon")
	case 1:
		success(header, "Found 1 matching addon")
		addons[0].showcase()
		fmt.Println()
	default:
		success(header, "Found "+strconv.Itoa(n)+" addons matching")
		for _, item := range addons {
			fmt.Println(" ", item.snippet(40))
		}
		fmt.Println()
	}
	return nil
}

func showRemote(url string) string {
	var screen = new(strings.Builder)

	screen.WriteString(" > ")

	m, err := fetchManifestAt(url)
	if err != nil {
		screen.WriteString(brush.Paint(brush.BrightWhite, brush.UseColor(brush.Red), " BROKEN ").String())
		screen.WriteByte(' ')
	}

	if strings.HasPrefix(url, "https://"+GITHUB_RAW_HOST+"/lite-xl/") {
		screen.WriteString(brush.Paint(
			brush.BrightWhite,
			brush.UseColor(brush.Green),
			" OFFICIAL ",
		).String())

		screen.WriteByte(' ')
	}

	if len(m.Addons) > 0 {
		screen.WriteString(brush.Paint(brush.Black, brush.UseColor(brush.BrightWhite), " ", len(m.Addons), " ADDONS ").String())
		counter := make([]int, len(aTypes))
		for _, a := range m.Addons {
			counter[a.AddonsType]++
		}
		for t, c := range counter {
			if c != 0 {
				icon := addonsType(t).icon()
				screen.WriteString(icon.Append(strconv.Itoa(c) + " ").String())
			}
		}
		screen.WriteString("\n   ")
	}

	screen.WriteString(url)

	return screen.String()
}
