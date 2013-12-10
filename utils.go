package main

import (
	"fmt"
	"os"
	"strings"
)

func str_in_slice(str string, slice []string) bool {
	for _, s := range slice {
		if str == s {
			return true
		}
	}
	return false
}

func handle_err(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "hwaf-gen-extpackdist: %v\n", err)
		os.Exit(1)
	}
}

func path_exists(name string) bool {
	_, err := os.Stat(name)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

func splitpath(p string) []string {
	return strings.Split(p, "/")
}

// commonpath returns the largest commonpath between p1 and p2
//  Example: /foo/bar/include /foo/barz => /foo
func commonpath(p1, p2 string) string {
	if len(p1) < len(p2) {
		p1, p2 = p2, p1
	}
	pp1 := splitpath(p1)
	pp2 := splitpath(p2)
	pp := make([]string, 0, len(pp2))
	// len(p1) >= len(p2)
	for i, p := range pp2 {
		if pp1[i] != pp2[i] {
			break
		}
		pp = append(pp, p)
	}
	return strings.Join(pp, "/")
}

// EOF
