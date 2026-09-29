//go:build windows
// +build windows

package main

// #cgo CFLAGS: -I${SRCDIR}/vendor/github.com/smacker/go-tree-sitter
// #cgo CFLAGS: -I${SRCDIR}/vendor/github.com/smacker/go-tree-sitter/tree_sitter
// #cgo LDFLAGS: -L${SRCDIR}/vendor/github.com/smacker/go-tree-sitter
import "C"
