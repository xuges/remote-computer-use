package app

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/xuges/remote-computer-use/internal/config"
	"github.com/xuges/remote-computer-use/internal/core"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var Version = "dev"

func flags(name string) *flag.FlagSet {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	return f
}
func emit(v any, j bool) {
	if j {
		_ = json.NewEncoder(os.Stdout).Encode(v)
		return
	}
	switch x := v.(type) {
	case core.Result:
		if x.Info != nil {
			i := *x.Info
			if x.Error != "" {
				i.Error = x.Error
			}
			emit(i, false)
		} else if x.OK {
			fmt.Fprintln(os.Stdout, "ok")
		} else {
			fmt.Fprintln(os.Stderr, x.Error)
		}
	case core.Info:
		fmt.Fprintf(os.Stdout, "%s\t%s\t%s", x.Name, x.Protocol, x.State)
		if x.Width > 0 {
			fmt.Fprintf(os.Stdout, "\t%dx%d", x.Width, x.Height)
		}
		if x.Error != "" {
			fmt.Fprintf(os.Stdout, "\t%s", x.Error)
		}
		fmt.Fprintln(os.Stdout)
	default:
		_ = json.NewEncoder(os.Stdout).Encode(v)
	}
}
func failure(e error, j bool) int { emit(core.Result{Error: core.Code(e)}, j); return 1 }

func Ctl(args []string) int {
	if len(args) > 0 && args[0] == "doctor" {
		return doctor(args[1:])
	}
	if len(args) == 1 && args[0] == "--worker" {
		return Worker()
	}
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println(Version)
		return 0
	}
	if len(args) == 0 || args[0] == "--help" {
		fmt.Println("remote-computer-ctl list|status NAME|connect NAME|disconnect NAME [--json] [--config PATH]\nremote-computer-ctl connect|disconnect --all [--timeout 30s]\nremote-computer-ctl doctor [--json]")
		return 0
	}
	op := args[0]
	args = args[1:]
	name := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name = args[0]
		args = args[1:]
	}
	f := flags("ctl")
	j := f.Bool("json", false, "")
	all := f.Bool("all", false, "")
	path := f.String("config", config.DefaultPath(), "")
	timeout := f.Duration("timeout", 30*time.Second, "")
	if f.Parse(args) != nil || f.NArg() != 0 || *timeout <= 0 || *timeout > 10*time.Minute {
		return failure(core.InvalidArgs, *j)
	}
	if op != "list" && op != "status" && op != "connect" && op != "disconnect" {
		return failure(core.InvalidArgs, *j)
	}
	if name != "" && !config.ValidName(name) {
		return failure(core.InvalidArgs, *j)
	}
	if (op == "list" && (name != "" || *all)) || (op == "status" && (name == "" || *all)) || ((op == "connect" || op == "disconnect") && ((name == "") == !*all)) {
		return failure(core.InvalidArgs, *j)
	}
	dir, e := runtimeDir()
	if e != nil {
		return failure(e, *j)
	}
	cfg, e := config.Load(*path)
	if e != nil && op != "disconnect" {
		if _, stat := os.Stat(*path); !os.IsNotExist(stat) || op == "connect" {
			return failure(e, *j)
		}
	}
	names := map[string]bool{}
	for n := range cfg.Connections {
		names[n] = true
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sock") || strings.HasSuffix(entry.Name(), ".json") {
			n := strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".sock"), ".json")
			if config.ValidName(n) {
				names[n] = true
			}
		}
	}
	if op == "status" {
		i := query(dir, name, cfg.Connections[name].Protocol)
		emit(i, *j)
		if i.State != "ready" {
			return 1
		}
		return 0
	}
	if op == "list" {
		ns := sorted(names)
		infos := make([]core.Info, 0, len(ns))
		for _, n := range ns {
			infos = append(infos, query(dir, n, cfg.Connections[n].Protocol))
		}
		if *j {
			emit(infos, true)
		} else {
			for _, i := range infos {
				emit(i, false)
			}
		}
		return 0
	}
	var ns []string
	if *all {
		if op == "connect" {
			m := map[string]bool{}
			for n := range cfg.Connections {
				m[n] = true
			}
			ns = sorted(m)
		} else {
			ns = sorted(names)
		}
	} else {
		ns = []string{name}
	}
	results := make([]core.Result, len(ns))
	var wg sync.WaitGroup
	gate := make(chan struct{}, 4)
	for k, n := range ns {
		wg.Add(1)
		go func(k int, n string) {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()
			if op == "connect" {
				c, ok := cfg.Connections[n]
				if !ok {
					results[k] = core.Result{Error: core.InvalidConfig}
				} else {
					results[k] = connect(dir, n, c, *timeout)
				}
			} else {
				lock, e := lockFile(filepath.Join(dir, n+".start.lock"))
				if e != nil {
					results[k] = core.Result{Error: core.Code(e)}
				} else {
					results[k] = stop(dir, n)
					lock.Close()
				}
			}
			if results[k].Info == nil {
				results[k].Info = &core.Info{Name: n, State: "failed", Error: results[k].Error}
			}
		}(k, n)
	}
	wg.Wait()
	code := 0
	for _, r := range results {
		if !r.OK {
			code = 1
		}
	}
	if *all && *j {
		emit(results, true)
	} else {
		for _, r := range results {
			emit(r, *j)
		}
	}
	return code
}
func sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func Use(args []string) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println(Version)
		return 0
	}
	if len(args) == 0 || args[0] == "--help" {
		fmt.Println("remote-computer-use --conn NAME screenshot --output PATH|-\nremote-computer-use --conn NAME move|click --x X --y Y [--button left|middle|right] [--count 1|2]\nremote-computer-use --conn NAME scroll --dx N --dy N\nremote-computer-use --conn NAME drag --from X,Y --to X,Y --duration 500ms\nremote-computer-use --conn NAME key CTRL+L\nremote-computer-use --conn NAME type --text TEXT|--stdin\nOptions: --json --timeout 10s --geometry-version N")
		return 0
	}
	// Both global and action flags are accepted on either side of the action name.
	op := ""
	chord := ""
	clean := []string{}
	valueFlags := map[string]bool{"--conn": true, "--timeout": true, "--geometry-version": true, "--output": true, "--x": true, "--y": true, "--dx": true, "--dy": true, "--button": true, "--count": true, "--from": true, "--to": true, "--duration": true, "--text": true, "--method": true}
	for k := 0; k < len(args); k++ {
		v := args[k]
		if valueFlags[v] {
			if k+1 >= len(args) {
				return failure(core.InvalidArgs, false)
			}
			clean = append(clean, v, args[k+1])
			k++
			continue
		}
		if strings.HasPrefix(v, "-") {
			clean = append(clean, v)
			continue
		}
		if op == "" {
			op = v
		} else if op == "key" && chord == "" {
			chord = v
		} else {
			return failure(core.InvalidArgs, false)
		}
	}
	f := flags("use")
	name := f.String("conn", "", "")
	j := f.Bool("json", false, "")
	timeout := f.Duration("timeout", 10*time.Second, "")
	geometry := f.Uint64("geometry-version", 0, "")
	output := f.String("output", "", "")
	x := f.Int("x", -1, "")
	y := f.Int("y", -1, "")
	dx := f.Int("dx", 0, "")
	dy := f.Int("dy", 0, "")
	button := f.String("button", "left", "")
	count := f.Int("count", 1, "")
	from := f.String("from", "", "")
	to := f.String("to", "", "")
	duration := f.Duration("duration", 500*time.Millisecond, "")
	text := f.String("text", "", "")
	stdin := f.Bool("stdin", false, "")
	method := f.String("method", "keys", "")
	if f.Parse(clean) != nil || f.NArg() != 0 || !config.ValidName(*name) || *timeout <= 0 || *timeout > time.Hour {
		return failure(core.InvalidArgs, *j)
	}
	allowed := map[string]string{
		"screenshot": " output ", "move": " x y ", "click": " x y button count ",
		"scroll": " dx dy ", "drag": " from to duration ", "key": " ",
		"type": " text stdin method ",
	}
	valid := true
	f.Visit(func(v *flag.Flag) {
		if !strings.Contains(" conn json timeout geometry-version "+allowed[op], " "+v.Name+" ") {
			valid = false
		}
	})
	if !valid {
		return failure(core.InvalidArgs, *j)
	}
	a := core.Action{Kind: op, X: *x, Y: *y, DX: *dx, DY: *dy, Button: *button, Count: *count, Duration: *duration, Chord: chord, Text: *text, Method: *method, Geometry: *geometry}
	switch op {
	case "screenshot":
		if *output == "" || (*output == "-" && *j) {
			return failure(core.InvalidArgs, *j)
		}
	case "move", "click", "scroll":
	case "drag":
		var e error
		a.X, a.Y, e = coordinates(*from)
		if e != nil {
			return failure(e, *j)
		}
		a.ToX, a.ToY, e = coordinates(*to)
		if e != nil {
			return failure(e, *j)
		}
	case "key":
		if _, e := parseChord(chord); e != nil {
			return failure(e, *j)
		}
	case "type":
		hasText := false
		f.Visit(func(v *flag.Flag) {
			if v.Name == "text" {
				hasText = true
			}
		})
		if hasText == *stdin {
			return failure(core.InvalidArgs, *j)
		}
		if *stdin {
			b, e := io.ReadAll(io.LimitReader(os.Stdin, (1<<20)+1))
			if e != nil || len(b) > 1<<20 {
				return failure(core.InvalidArgs, *j)
			}
			a.Text = string(b)
		}
	default:
		return failure(core.InvalidArgs, *j)
	}
	dir, e := runtimeDir()
	if e != nil {
		return failure(e, *j)
	}
	r := exchange(dir, *name, core.Request{Operation: "action", Action: a, Timeout: *timeout})
	if !r.OK {
		emit(r, *j)
		return 1
	}
	if op == "screenshot" {
		if *output == "-" {
			if writeAll(os.Stdout, r.PNG) != nil {
				return 1
			}
			return 0
		}
		if e = atomicWrite(*output, r.PNG); e != nil {
			return failure(e, *j)
		}
		r.PNG = nil
		r.Output = *output
	}
	emit(r, *j)
	return 0
}

func coordinates(s string) (int, int, error) {
	p := strings.Split(s, ",")
	if len(p) != 2 {
		return 0, 0, core.InvalidArgs
	}
	x, e := strconv.Atoi(p[0])
	if e != nil {
		return 0, 0, core.InvalidArgs
	}
	y, e := strconv.Atoi(p[1])
	if e != nil {
		return 0, 0, core.InvalidArgs
	}
	return x, y, nil
}
func atomicWrite(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".rcu-image-")
	if e != nil {
		return core.IOError
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = io.Copy(f, bytes.NewReader(b)); e != nil {
		f.Close()
		return core.IOError
	}
	if f.Close() != nil {
		return core.IOError
	}
	if os.Rename(tmp, path) != nil {
		return core.IOError
	}
	return nil
}
