package cliutil

import "strings"

// ReorderArgs moves flag tokens ahead of positional arguments so the standard
// library's flag package parses them. Go's flag.Parse STOPS at the first
// non-flag argument, so "recalls pacemaker --json" would silently ignore
// --json. Every command has positional args (a search term, a device id), so
// without this, flags placed after them are dropped — which broke --json and
// --class validation in live testing.
//
// valueFlags names the flags that consume a following value in "--flag value"
// form (e.g. "class", "limit"); their value token travels with the flag. The
// "--flag=value" form needs no entry. Order among flags and among positionals
// is preserved.
func ReorderArgs(args []string, valueFlags map[string]bool) []string {
	flags := make([]string, 0, len(args))
	positional := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			// Everything after "--" is positional by convention.
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			name := strings.TrimLeft(strings.SplitN(a, "=", 2)[0], "-")
			if valueFlags[name] && !strings.Contains(a, "=") && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}
