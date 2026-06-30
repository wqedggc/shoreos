package web

import "embed"

// StaticFS is embedded into the Go binary so the FIRE frontend deploys with the API.
//
//go:embed static/*
var StaticFS embed.FS
