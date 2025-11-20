// file: web_embed.go
// version: 1.0.0
// guid: 4d5e6f7a-8b9c-0d1e-2f3a-4b5c6d7e8f9a
//go:build embed_frontend

package main

import "embed"

//go:embed web/dist
var WebFS embed.FS
