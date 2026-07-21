package ui

import (
	"fmt"
	"strings"

	"github.com/Gelmezon/grok-switch/internal/ui/theme"
)

const bannerArt = `
  ██████╗ ██████╗  ██████╗ ██╗  ██╗  ███████╗██╗    ██╗██╗████████╗ ██████╗██╗  ██╗
 ██╔════╝ ██╔══██╗██╔═══██╗██║ ██╔╝  ██╔════╝██║    ██║██║╚══██╔══╝██╔════╝██║  ██║
 ██║  ███╗██████╔╝██║   ██║█████╔╝   ███████╗██║ █╗ ██║██║   ██║   ██║     ███████║
 ██║   ██║██╔══██╗██║   ██║██╔═██╗   ╚════██║██║███╗██║██║   ██║   ██║     ██╔══██║
 ╚██████╔╝██║  ██║╚██████╔╝██║  ██╗  ███████║╚███╔███╔╝██║   ██║   ╚██████╗██║  ██║
  ╚═════╝ ╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═╝  ╚══════╝ ╚══╝╚══╝ ╚═╝   ╚═╝    ╚═════╝╚═╝  ╚═╝
`

// Banner returns the ASCII brand banner with version subtitle.
func Banner(version string) string {
	art := strings.TrimRight(bannerArt, "\n")
	sub := fmt.Sprintf("  Grok Build 中间站切换工具  v%s              by DavidZhao", version)
	if theme.Enabled() {
		return theme.Primary.Render(art) + "\n" + theme.Muted.Render(sub) + "\n"
	}
	return art + "\n" + sub + "\n"
}

// HeaderLine is a compact one-line brand header for CLI cards.
func HeaderLine(title, version string) string {
	left := fmt.Sprintf("grok-switch  %s", title)
	if version == "" {
		if theme.Enabled() {
			return theme.Title.Render(left)
		}
		return left
	}
	right := "v" + version
	if theme.Enabled() {
		return theme.Title.Render(left) + "  " + theme.Muted.Render(right)
	}
	return left + "  " + right
}
