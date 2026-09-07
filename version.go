package main

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

func writeVersion(out io.Writer) error {
	info, _ := debug.ReadBuildInfo()
	_, err := io.WriteString(out, describeBuild(info, runtime.Version()))
	return err
}

func describeBuild(info *debug.BuildInfo, goVersion string) string {
	version, commit := "dev", "unknown"
	modified := false
	if info != nil {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				commit = setting.Value
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
	}
	if modified {
		commit += " (modified)"
	}
	return fmt.Sprintf("firecheck %s\nCommit: %s\nGo: %s\n", version, commit, goVersion)
}
