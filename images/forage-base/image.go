// Package image embeds the forage-base Dockerfile for local builds.
package image

import _ "embed"

// Dockerfile is the forage-base Dockerfile content, embedded at compile time.
//
//go:embed Dockerfile
var Dockerfile []byte
