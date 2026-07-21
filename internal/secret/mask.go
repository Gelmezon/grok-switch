package secret

// MaskSecret returns a masked representation of a secret string.
// Values of length <= 8 become "****"; longer values show first/last 4 chars.
func MaskSecret(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "…" + s[len(s)-4:]
}
