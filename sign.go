package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/subcommands"
)

type signCmd struct {
	Identity     string
	Entitlements string
}

func (*signCmd) Name() string {
	return "sign"
}

func (*signCmd) Synopsis() string {
	return "Sign an app bundle"
}

func (*signCmd) Usage() string {
	return `sign [-i identity][-e entitlements] some.app`
}

func (c *signCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	for _, arg := range f.Args() {
		if err := c.signApp(arg); err != nil {
			errPrintf("error signing %s: %v\n", arg, err)
			return subcommands.ExitFailure
		}
	}
	return subcommands.ExitSuccess
}

func (c *signCmd) SetFlags(f *flag.FlagSet) {
	f.StringVar(&c.Identity, "i", "Developer ID", "Identity to sign the app")
	f.StringVar(&c.Entitlements, "e", "", "Custom entitlements to use")
}

func (c *signCmd) signApp(p string) error {
	var toSign []string
	err := filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		ext := filepath.Ext(path)
		if info.IsDir() {
			if ext == ".app" || ext == ".framework" || ext == ".xpc" {
				toSign = append(toSign, path)
			}
		} else {
			if ext == ".dylib" {
				toSign = append(toSign, path)
			} else {
				if filepath.Base(filepath.Dir(path)) == "Helpers" {
					toSign = append(toSign, path)
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Sign entries in reverse order, since inner bundles need to
	// be signed before outer ones
	for ii := len(toSign) - 1; ii >= 0; ii-- {
		if err := c.signEntry(p, toSign[ii]); err != nil {
			return err
		}
	}
	return nil
}

func (c *signCmd) signEntry(root, p string) error {
	rel, _ := filepath.Rel(root, p)
	verbosePrintf(1, "signing %s\n", rel)
	var args []string
	if *verbose > 0 {
		args = append(args, "--verbose")
	}
	args = append(args, "--force", "--options=runtime", "--timestamp")
	if c.Entitlements != "" {
		args = append(args, "--entitlements", c.Entitlements)
	}
	args = append(args, "--sign", c.Identity, p)
	cmd := exec.Command("codesign", args...)
	if *dryRun {
		fmt.Printf("%s\n", strings.Join(cmd.Args, " "))
		return nil
	}
	verbosePrintf(2, "%s\n", strings.Join(cmd.Args, " "))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
