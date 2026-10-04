package runner

import "path/filepath"

func join(elem ...string) string { return filepath.Join(elem...) }
func dir(path string) string     { return filepath.Dir(path) }
