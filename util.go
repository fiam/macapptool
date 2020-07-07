package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func isExecutable(info os.FileInfo) bool {
	return info.Mode()&0111 != 0
}

func verifySignature(p string) error {
	var args []string
	if *verbose > 0 {
		args = append(args, "--verbose=10")
	}
	args = append(args, "--assess")
	args = append(args, "--ignore-cache", "--no-cache")
	if strings.ToLower(filepath.Ext(p)) == ".app" {
		args = append(args, "--type", "execute")
	} else {
		args = append(args, "--type", "open", "--context", "context:primary-signature")
	}
	args = append(args, p)
	cmd := exec.Command("spctl", args...)
	if *dryRun {
		fmt.Printf("%s\n", strings.Join(cmd.Args, " "))
		return nil
	}
	verbosePrintf(2, "%s\n", strings.Join(cmd.Args, " "))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
