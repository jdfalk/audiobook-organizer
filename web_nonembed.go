// file: web_nonembed.go
// version: 1.0.0
// guid: 5e6f7a8b-9c0d-1e2f-3a4b-5c6d7e8f9a0b
//go:build !embed_frontend

package main

import "embed"

// WebFS is an empty embed.FS when not embedding frontend
var WebFS embed.FS
