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
	cReset  = "\033[0m"
	cBold   = "\033[1m"
	cDim    = "\033[2m"
	cRed    = "\033[31m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cGold   = "\033[38;5;179m"
)

func paint(c, s string) string {
	if !useColor {
		return s
	}
	return c + s + cReset
}

var shield = []string{
	"▗▖                  ▗▎",
	"▐█▃▁    ▁▂▂▂▂▁    ▁▃█▌",
	"▐████▇▆▇██████▇▆▇████▋",
	"▐████████████████████▋",
	"▐████████████████████▍",
	"▕████████████████████▏",
	" ▜██████████████████▋",
	" ▕██████████████████▏",
	"  ▝████████████████▘",
	"   ▝██████████████▘",
	"     ▀█▜██████▛█▀",
	"        ▀████▀",
	"          ▀▀",
}

func runeLen(s string) int { return len([]rune(s)) }

// printSideBySide prints the gold shield on the left and status rows on the right (cilium-style).
func printSideBySide(rows []string) {
	width := 0
	for _, l := range shield {
		if w := runeLen(l); w > width {
			width = w
		}
	}
	n := len(shield)
	if len(rows) > n {
		n = len(rows)
	}
	for i := 0; i < n; i++ {
		left := ""
		if i < len(shield) {
			left = shield[i]
		}
		pad := width - runeLen(left)
		line := " " + paint(cGold, left) + strings.Repeat(" ", pad) + "   "
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
