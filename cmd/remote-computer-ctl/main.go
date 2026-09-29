package main

import (
	"github.com/xuges/remote-computer-use/internal/app"
	"os"
)

func main() { os.Exit(app.Ctl(os.Args[1:])) }
