package cmd

import (
	"fmt"
	"runtime"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	// Version holds the current semantic version of ops-go
	Version = "0.1.0-alpha"
	// GitCommit holds the git commit sha injected at build time
	GitCommit = "dev"
	// BuildDate holds the RFC3339 build date injected at build time
	BuildDate = "unknown"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "显示 ops-go 版本与运行环境信息",
		Run: func(cmd *cobra.Command, args []string) {
			out := cmd.OutOrStdout()
			pterm.SetDefaultOutput(out)

			panel1 := pterm.DefaultBox.WithTitle("ops-go").Sprint(
				fmt.Sprintf("Version:    %s\nCommit:     %s\nBuild Date: %s", Version, GitCommit, BuildDate),
			)
			panel2 := pterm.DefaultBox.WithTitle("Runtime").Sprint(
				fmt.Sprintf("Go:         %s\nOS/Arch:    %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH),
			)

			_ = pterm.DefaultPanel.WithPanels([][]pterm.Panel{
				{{Data: panel1}, {Data: panel2}},
			}).Render()
		},
	}
}
