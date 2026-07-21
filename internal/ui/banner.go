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
	sub := fmt.Sprintf("  Grok Build 中间站切换工具  %s              by DavidZhao", VersionLabel(version))
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
	right := VersionLabel(version)
	if theme.Enabled() {
		return theme.Title.Render(left) + "  " + theme.Muted.Render(right)
	}
	return left + "  " + right
}

// VersionLabel returns a display version with exactly one leading v.
func VersionLabel(version string) string {
	if version == "dev" {
		return version
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}
