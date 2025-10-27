package sqs_gui

import "embed"

//go:embed all:dist
var Dist embed.FS

//go:embed all:templates
var Templates embed.FS
