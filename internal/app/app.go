package app

import "fmt"

// Run 是当前应用装配层的最小入口。
// 现在它还不执行 agent，只负责把 CLI 入口稳定下来。
func Run(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("arguments are not supported yet: %v", args)
	}

	return nil
}
