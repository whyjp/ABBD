//go:build !windows

package model

// osLocale is "" off Windows, where the environment names the locale.
func osLocale() string { return "" }
