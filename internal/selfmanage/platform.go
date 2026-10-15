package selfmanage

import "runtime"

func hostGOOS() string   { return runtime.GOOS }
func hostGOARCH() string { return runtime.GOARCH }
