// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

var useColor = detectColor()

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("SPHRAGIS_NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" || os.Getenv("CLICOLOR_FORCE") != "" {
		return true
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

const (
	cReset   = "\033[0m"
	cBold    = "\033[1m"
	cDim     = "\033[2m"
	cRed     = "\033[31m"
	cGreen   = "\033[32m"
	cYellow  = "\033[33m"
	cCream   = "\033[38;5;230m"
	cLogoRed = "\033[38;5;203m"
)

func paint(c, s string) string {
	if !useColor {
		return s
	}
	return c + s + cReset
}

// logoArt is the seal: a dashed cream ring around redaction bars (middle bar red).
// o = ring dot, C = cream bar cell, R = red bar cell, space = blank.
var logoArt = []string{
	"       o o o o       ",
	"    o o       o o    ",
	"   o             o   ",
	"  o               o  ",
	" o     CCCCCCC     o ",
	" o                 o ",
	"o    RRRRRRRRRRR    o",
	" o   RRRRRRRRRRR   o ",
	" o                 o ",
	"  o    CCCCCCC    o  ",
	"   o             o   ",
	"    o o       o o    ",
	"       o o o o       ",
}

func runeLen(s string) int { return len([]rune(s)) }

// paintLogo renders one logo line, coloring the ring/cream bars and the red bar.
func paintLogo(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'o':
			b.WriteString(paint(cCream, "·"))
		case 'C':
			b.WriteString(paint(cCream, "█"))
		case 'R':
			b.WriteString(paint(cLogoRed, "█"))
		default:
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// printSideBySide prints the seal logo on the left and status rows on the right (cilium-style).
func printSideBySide(rows []string) {
	width := 0
	for _, l := range logoArt {
		if w := runeLen(l); w > width {
			width = w
		}
	}
	n := len(logoArt)
	if len(rows) > n {
		n = len(rows)
	}
	for i := 0; i < n; i++ {
		src := ""
		if i < len(logoArt) {
			src = logoArt[i]
		}
		pad := width - runeLen(src)
		line := " " + paintLogo(src) + strings.Repeat(" ", pad) + "   "
		if i < len(rows) {
			line += rows[i]
		}
		fmt.Println(strings.TrimRight(line, " "))
	}
}

// row formats an aligned "Label:  value" status row.
func row(label, value string) string {
	return fmt.Sprintf("%-13s %s", label+":", value)
}

// humanInt formats an unsigned int with thousands separators.
func humanInt(n uint64) string {
	s := strconv.FormatUint(n, 10)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}
