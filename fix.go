package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/subcommands"
)

type fixCmd struct {
}

func (*fixCmd) Name() string {
	return "fix"
}

func (*fixCmd) Synopsis() string {
	return "Fix invalid symlinks and unsealed resources within an app bundle"
}

func (*fixCmd) Usage() string {
	return fmt.Sprintf(`Usage: %s fix some.app

	fix checks the app bundle for invalid structures
	(like unsealed resources) and fixes them automatically.
`, filepath.Base(os.Args[0]))
}

func (p *fixCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	for _, arg := range f.Args() {
		if err := p.fixApp(arg); err != nil {
			errPrintf("error fixing %s: %v\n", arg, err)
			return subcommands.ExitFailure
		}
	}
	return subcommands.ExitSuccess
}

func (p *fixCmd) SetFlags(f *flag.FlagSet) {
}

func (p *fixCmd) fixApp(path string) error {
	var frameworks []string
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == ".DS_Store" {
			if err := p.removeEntry(path); err != nil {
				return err
			}
		}
		if info.IsDir() && filepath.Ext(path) == ".framework" {
			frameworks = append(frameworks, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, f := range frameworks {
		if err := p.fixFramework(f); err != nil {
			return err
		}
	}
	return nil
}

func (p *fixCmd) removeEntry(path string) error {
	verbosePrintf(1, "removing %s\n", path)
	return osRemove(path)
}

func (p *fixCmd) fixFramework(path string) error {
	// Make sure we have no unsealed resources
	entries, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if (e.Mode() & os.ModeSymlink) != 0 {
			// Symlinks are always allowed
			continue
		}
		if e.IsDir() && e.Name() == "Versions" {
			// Versions is the only root entry allowed
			continue
		}
		// This resource needs to be sealed
		if err := p.sealResource(path, filepath.Join(path, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (p *fixCmd) sealResource(rootPath, resource string) error {
	verbosePrintf(1, "sealing %s\n", resource)
	rel, err := filepath.Rel(rootPath, resource)
	if err != nil {
		return err
	}
	dest := filepath.Join(rootPath, "Versions", "Current", rel)
	verbosePrintf(2, "moving %s to %s and symlinking\n", resource, dest)
	if err := osRename(resource, dest); err != nil {
		return err
	}
	return symlink(dest, resource)
}
