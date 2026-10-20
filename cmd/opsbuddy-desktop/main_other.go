//go:build !windows && !linux

package main

import "fmt"

func main() { fmt.Println("OpsBuddy desktop supports Windows and Linux.") }
