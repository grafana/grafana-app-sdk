package main

// These are for pretty printing in the logs
const (
	nc     = "\033[0m"
	red    = "\033[1;31m"
	green  = "\033[1;32m"
	yellow = "\033[1;33m"
	blue   = "\033[1;34m"
)

type OpinionatedModel struct {
	Number int    `json:"numField"`
	String string `json:"stringField"`
}
