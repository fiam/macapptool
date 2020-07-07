package main

import (
	"context"
	"flag"
	"fmt"
	"macapptool/internal/plist"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/subcommands"
)

type zipCmd struct {
	IncludeMacOSSuffix bool
	Output             string
	Delete             bool
	Force              bool
}

func (*zipCmd) Name() string {
	return "zip"
}

func (*zipCmd) Synopsis() string {
	return "Create a zip file from an app bundle"
}

func (*zipCmd) Usage() string {
	return `zip [-o output][-m][-d][-f] some.app
`
}

func (c *zipCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if f.NArg() != 1 {
		return subcommands.ExitUsageError
	}
	appPath := strings.TrimSuffix(f.Arg(0), "/")
	if err := c.zipFile(appPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func (c *zipCmd) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&c.IncludeMacOSSuffix, "m", true, "Include macOS suffix in the default output filename")
	f.BoolVar(&c.Delete, "d", false, "Delete original .app bundle after zipping")
	f.BoolVar(&c.Force, "f", false, "Overwrite output file if it exists")
	f.StringVar(&c.Output, "o", "", "Output filename. Defaults to App_version_macOS.zip")
}

func (c *zipCmd) zipFile(appPath string) error {
	output := c.Output
	if output == "" {
		var err error
		output, err = c.outputFilename(appPath)
		if err != nil {
			return err
		}
	}
	if st, err := os.Stat(output); err == nil && !st.IsDir() {
		if !c.Force {
			return fmt.Errorf("%s already exists", output)
		}
		if *dryRun {
			fmt.Printf("rm %s\n", output)
		} else {
			verbosePrintf(1, "removing %s\n", output)
			if err := os.Remove(output); err != nil {
				return fmt.Errorf("error removing %s: %v", output, err)
			}
		}
	}
	args := []string{"ditto",
		"-c", "-k",
		"--norsrc",
		"--sequesterRsrc",
		"--keepParent",
		appPath, output}
	if *dryRun || *verbose > 0 {
		cmdString := commandDebugString(args...)
		fmt.Printf("@%s\n", cmdString)
	}
	if !*dryRun {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	if c.Delete {
		if *dryRun || *verbose > 0 {
			fmt.Printf("rm -r %s\n", appPath)
		}
		if !*dryRun {
			if err := os.RemoveAll(appPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *zipCmd) outputFilename(appPath string) (string, error) {
	infoPlistPath := filepath.Join(appPath, "Contents", "Info.plist")
	f, err := os.Open(infoPlistPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	plist, err := plist.New(f)
	if err != nil {
		return "", err
	}
	name, err := plist.BundleName()
	if err != nil {
		return "", err
	}
	version, err := plist.BundleShortVersionString()
	if err != nil {
		return "", err
	}
	macOSSuffix := ""
	if c.IncludeMacOSSuffix {
		macOSSuffix = "_macOS"
	}
	return fmt.Sprintf("%s_%s%s.zip", name, version, macOSSuffix), nil

}
