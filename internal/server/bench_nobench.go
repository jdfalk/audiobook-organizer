// file: internal/server/bench_nobench.go
// version: 1.0.0
// guid: 4d5e6f7a-8b9c-0123-def0-444444444444

//go:build !bench

package server

import "github.com/gin-gonic/gin"

// setupBenchRoutes is a no-op when the bench build tag is not set.
func (s *Server) setupBenchRoutes(_ *gin.RouterGroup) {}
