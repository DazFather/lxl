package main

import (
	"fmt"
	"github.com/DazFather/brush"
	"strings"
)

const USAGE = "Usage: lxl <install|uninstall|find> <pluginID>"

// Palette
var (
	warn    = newPrinter(brush.Yellow, " ! ")
	danger  = newPrinter(brush.Red, " X ")
	success = newPrinter(brush.Green, " v ")
)

func newPrinter(baseTone brush.ANSIColor, prefix string) func(...any) {
	black := brush.New(brush.BrightWhite, brush.UseColor(baseTone))
	white := brush.New(brush.Black, brush.UseColor(baseTone+8))

	return func(v ...any) {
		var suffix string = "\n"
		if len(v) >= 2 {
			suffix = brush.Paintln(baseTone, nil, v[1:]...).String()
			v[0] = fmt.Sprint(" ", v[0], " ")
			v = v[0:1]
		}
		fmt.Printf("%s%s %s", black.Paint(prefix), white.Paint(v...), suffix)
	}
}

func (t addonsType) color() brush.ANSIColor {
	return brush.ANSIColor(t) + 9
}

func (t addonsType) icon() brush.Painted {
	s := t.String()
	ch := " " + strings.ToUpper(s[:1]) + " "

	return brush.Paint(brush.White, brush.UseColor(t.color()), ch)
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

func (a addon) showcase() brush.Highlighted {
	color := a.AddonsType.color()

	return brush.Join(
		brush.Paint(brush.White, brush.UseColor(color), " ", strings.ToUpper(a.AddonsType.String()), " "),
		brush.Paint(color, nil, "\t" + a.ID),
		"\tv. ", a.Version,
		"\nDescription: ", a.Description,
		"\n\nTo install last version use command:\n ",
		brush.Paint(brush.BrightWhite, brush.UseColor(brush.BrightBlack), " lxl install ", a.ID, " "),
		"\n",
	)
}
