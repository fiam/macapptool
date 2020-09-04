package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
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

func (c *signCmd) signPath(root, p string) error {
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	ext := filepath.Ext(p)
	var shouldSign bool
	if st.IsDir() {
		shouldSign = ext == ".app" || ext == ".framework" || ext == ".xpc"
		// Inner bundles need to be signed before outer ones
		entries, err := ioutil.ReadDir(p)
		if err != nil {
			return err
		}
		for _, v := range entries {
			if err := c.signPath(root, filepath.Join(p, v.Name())); err != nil {
				return err
			}
		}
	} else {
		shouldSign = ext == ".dylib" || filepath.Base(filepath.Dir(p)) == "Helpers" || isExecutable(st)
	}
	if shouldSign {
		return c.signEntry(root, p)
	}
	return nil
}

func (c *signCmd) signApp(p string) error {
	// If the argument is foo.app/,
	// filepath.Ext() will return an empty
	// string. Make sure we don't skip it
	if strings.HasSuffix(p, "/") {
		p = p[:len(p)-1]
	}
	if err := c.signPath(p, p); err != nil {
		return err
	}
	// Verify signature
	ext := strings.ToLower(filepath.Ext(p))
	if ext == ".app" || ext == ".framework" {
		return verifySignature(p)
	}
	return nil
}

func (c *signCmd) signEntry(root, p string) error {
	rel, _ := filepath.Rel(root, p)
	name := rel
	if name == "" {
		name = root
	}
	verbosePrintf(1, "signing %s\n", name)
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
