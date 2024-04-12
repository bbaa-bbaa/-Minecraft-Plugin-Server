package core

import "regexp"

var DedicatedServerMessage = regexp.MustCompile(`\[.*\]: (.*)$`)
var PlayerMessage = regexp.MustCompile(`\]\: <.*?>.*`)
var GameLeftMessage = regexp.MustCompile(`\w+ (left|joined) the game`)
var LoginMessage = regexp.MustCompile(`\[.*\]:.*? logged in with`)
