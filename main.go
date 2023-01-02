package main

import (
	"pinger/app"
	_ "pinger/app/uping"
	_ "pinger/app/userver"
	_ "pinger/app/watcher"
)

func main() {
	_ = app.RootCmd().Execute()
}
