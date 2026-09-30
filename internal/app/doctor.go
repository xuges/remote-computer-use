package app

import (
	"fmt"
	"os"
	"runtime"

	"github.com/xuges/remote-computer-use/internal/backend"
	"github.com/xuges/remote-computer-use/internal/core"
	"github.com/xuges/remote-computer-use/internal/runtimebundle"
)

func doctor(args []string) int {
	f := flags("doctor")
	j := f.Bool("json", false, "")
	if f.Parse(args) != nil || f.NArg() != 0 {
		return failure(core.InvalidArgs, *j)
	}
	type check struct {
		Protocol string     `json:"protocol"`
		OK       bool       `json:"ok"`
		Error    core.Error `json:"error,omitempty"`
	}
	result := struct {
		OK       bool    `json:"ok"`
		Version  string  `json:"version"`
		Platform string  `json:"platform"`
		Runtime  string  `json:"runtime"`
		Checks   []check `json:"checks"`
	}{OK: true, Version: Version, Platform: runtime.GOOS + "/" + runtime.GOARCH, Runtime: "external"}
	if runtimebundle.Embedded() {
		result.Runtime = "embedded"
	}
	if os.Getenv("REMOTE_COMPUTER_LIB_DIR") != "" {
		result.Runtime = "override"
	}
	for _, p := range []string{"vnc", "rdp"} {
		c := check{Protocol: p, OK: true}
		if err := backend.Check(p); err != nil {
			c.OK = false
			c.Error = core.Code(err)
			result.OK = false
		}
		result.Checks = append(result.Checks, c)
	}
	if *j {
		emit(result, true)
	} else {
		fmt.Printf("%s %s (runtime: %s)\n", result.Version, result.Platform, result.Runtime)
		for _, c := range result.Checks {
			if c.OK {
				fmt.Printf("%s: ready\n", c.Protocol)
			} else {
				fmt.Printf("%s: %s\n", c.Protocol, c.Error)
			}
		}
		if !result.OK {
			fmt.Println("Check the platform/glibc requirement, writable executable cache filesystem, and REMOTE_COMPUTER_LIB_DIR override. See docs/installation.md.")
		}
	}
	if !result.OK {
		return 1
	}
	return 0
}
