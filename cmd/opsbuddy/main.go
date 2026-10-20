// Deprecated compatibility entry. OpsBuddy is distributed only as a desktop app.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "OpsBuddy 的独立网页服务已停用。请运行 start.cmd 或 release/OpsBuddy/OpsBuddy.exe 启动桌面版。")
	os.Exit(1)
}
