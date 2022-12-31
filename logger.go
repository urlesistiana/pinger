package main

import (
	"log"
	"os"
)

var logWarn = log.New(os.Stderr, "warn: ", log.LstdFlags|log.Lmsgprefix)
var logInfo = log.New(os.Stdout, "info: ", log.LstdFlags|log.Lmsgprefix)
